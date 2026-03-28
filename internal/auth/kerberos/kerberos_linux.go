//go:build linux

package kerberos

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"sync"

	"github.com/sassoftware/gssapi"
)

var (
	lib      *gssapi.Lib
	libMutex sync.RWMutex
)

// Init loads the system GSSAPI library via dlopen.
// This should be called once at application startup.
// If the library cannot be loaded, Available() will return false.
func Init() error {
	libMutex.Lock()
	defer libMutex.Unlock()

	if lib != nil {
		return nil
	}

	var err error
	// Use versioned library name (.so.2) which is provided by krb5-libs runtime package.
	// The unversioned .so is only in krb5-devel.
	lib, err = gssapi.Load(&gssapi.Options{
		LibPath: "libgssapi_krb5.so.2",
	})
	if err != nil {
		return fmt.Errorf("failed to load GSSAPI library: %w", err)
	}

	return nil
}

// Close unloads the GSSAPI library.
func Close() {
	libMutex.Lock()
	defer libMutex.Unlock()

	if lib != nil {
		_ = lib.Unload()
		lib = nil
	}
}

// Available returns true if the GSSAPI library was successfully loaded.
func Available() bool {
	libMutex.RLock()
	defer libMutex.RUnlock()
	return lib != nil
}

// GetSPNEGOToken acquires Kerberos credentials from the system's default
// credential cache and generates a SPNEGO token for the given service
// principal name. Returns the base64-encoded token string.
//
// The spn parameter should be in the format "HTTP@hostname" (not "HTTP/hostname").
//
// This function acquires fresh credentials on each call, so if the user renews
// their Kerberos ticket via kinit, the application will automatically use the
// new ticket without needing a restart.
func GetSPNEGOToken(spn string) (string, error) {
	libMutex.RLock()
	l := lib
	libMutex.RUnlock()

	if l == nil {
		return "", fmt.Errorf("GSSAPI library not initialized")
	}

	spnBuffer, err := l.MakeBufferString(spn)
	if err != nil {
		return "", fmt.Errorf("failed to create SPN buffer: %w", err)
	}
	defer func() { _ = spnBuffer.Release() }()

	serviceName, err := spnBuffer.Name(l.GSS_C_NT_HOSTBASED_SERVICE)
	if err != nil {
		return "", fmt.Errorf("failed to create service name: %w", err)
	}
	defer func() { _ = serviceName.Release() }()

	mechSet, err := l.MakeOIDSet(l.GSS_MECH_SPNEGO)
	if err != nil {
		return "", fmt.Errorf("failed to create mechanism set: %w", err)
	}
	defer func() { _ = mechSet.Release() }()

	cred, actualMechs, _, err := l.AcquireCred(nil, 0, mechSet, gssapi.GSS_C_INITIATE)
	if err != nil {
		return "", fmt.Errorf("failed to acquire credentials: %w", err)
	}
	defer func() { _ = cred.Release() }()
	if actualMechs != nil {
		defer func() { _ = actualMechs.Release() }()
	}

	ctx, actualMech, token, _, _, err := l.InitSecContext(
		cred,
		nil,
		serviceName,
		l.GSS_MECH_SPNEGO,
		gssapi.GSS_C_MUTUAL_FLAG|gssapi.GSS_C_REPLAY_FLAG|gssapi.GSS_C_SEQUENCE_FLAG,
		0,
		l.GSS_C_NO_CHANNEL_BINDINGS,
		nil,
	)
	if err != nil && !errors.Is(err, gssapi.ErrContinueNeeded) {
		return "", fmt.Errorf("failed to initialize security context: %w", err)
	}
	if ctx != nil {
		defer func() { _ = ctx.Release() }()
	}
	if actualMech != nil {
		defer func() { _ = actualMech.Release() }()
	}
	if token != nil {
		defer func() { _ = token.Release() }()
	}

	if token == nil || len(token.Bytes()) == 0 {
		return "", fmt.Errorf("no token generated")
	}

	return base64.StdEncoding.EncodeToString(token.Bytes()), nil
}

// SetSPNEGOHeader generates a SPNEGO token and sets it as an
// "Authorization: Negotiate <token>" header on the request.
func SetSPNEGOHeader(req *http.Request, spn string) error {
	token, err := GetSPNEGOToken(spn)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Negotiate "+token)
	return nil
}

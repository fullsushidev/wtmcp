"""Root conftest for plugin tests.

Ensures each plugin's test modules import from the correct plugin
directory, preventing cross-plugin collisions when multiple plugins
have identically-named files (e.g., handler.py).

Uses pytest_collect_file to evict stale modules before each test
file is imported, so `import handler` at module level resolves
to the correct plugin.
"""

import sys
from pathlib import Path

_plugins_dir = Path(__file__).resolve().parent

# Track which plugin dir is currently active so we only evict
# when switching between plugins.
_active_plugin = None


def _plugin_dir_for(fspath):
    """Return the plugin directory for a path, or None."""
    try:
        rel = Path(fspath).resolve().relative_to(_plugins_dir)
    except ValueError:
        return None
    parts = rel.parts
    if len(parts) >= 2:
        return str(_plugins_dir / parts[0])
    return None


def pytest_collect_file(parent, file_path):
    """Called before each test file is collected/imported.
    Evict cached modules from other plugins so import handler
    resolves correctly."""
    global _active_plugin  # noqa: PLW0603

    plugin_dir = _plugin_dir_for(file_path)
    if plugin_dir is None:
        return None

    if plugin_dir == _active_plugin:
        return None  # same plugin, no switch needed

    _active_plugin = plugin_dir

    # Evict all bare-name modules from the plugins tree
    # so they get re-imported from the new plugin dir.
    for name in list(sys.modules):
        mod = sys.modules[name]
        mod_file = getattr(mod, "__file__", "") or ""
        if not mod_file:
            continue
        plugins_posix = _plugins_dir.as_posix()
        if plugins_posix in mod_file and plugin_dir not in mod_file:
            del sys.modules[name]

    # Ensure the correct plugin dir is first on sys.path
    if plugin_dir in sys.path:
        sys.path.remove(plugin_dir)
    sys.path.insert(0, plugin_dir)

    return None  # let default collection proceed

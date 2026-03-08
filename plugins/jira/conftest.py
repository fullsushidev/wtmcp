"""pytest conftest: add plugin directory to sys.path for module resolution."""

import sys
from pathlib import Path

sys.path.insert(0, str(Path(__file__).parent))

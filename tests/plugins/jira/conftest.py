"""pytest conftest: add plugin directory to sys.path for module resolution."""

import sys
from pathlib import Path

# Resolve to plugins/jira/ from tests/plugins/jira/
_project_root = Path(__file__).parent.parent.parent.parent
sys.path.insert(0, str(_project_root / "plugins" / "jira"))

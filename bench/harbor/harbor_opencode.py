"""Offline opencode agent for the bench net.

Harbor's stock OpenCode.install() installs Node via nvm — it curls
raw.githubusercontent.com and nodejs.org, both unreachable from the bench's
task containers (Artifactory-only egress). The task images prebake node+npm
and their .npmrc already points at the Artifactory npm proxy; the token rides
in by NAME only (${NPM_AUTH_TOKEN}). This class is the stock agent minus the
internet branch.

Load it with:  agents: name harbor_opencode:HarborOpenCode  (PYTHONPATH=bench/harbor)
"""

from __future__ import annotations

from harbor.agents.installed.opencode import OpenCode
from harbor.environments.base import BaseEnvironment


class HarborOpenCode(OpenCode):
    """OpenCode, Artifactory-only: node+npm are prebaked; install via the proxy."""

    async def install(self, environment: BaseEnvironment) -> None:
        version_spec = f"@{self._version}" if self._version else "@latest"
        await self.exec_as_agent(
            environment,
            command=(
                "set -euo pipefail; "
                "node --version && npm --version && "
                f"npm i -g opencode-ai{version_spec} && "
                "opencode --version"
            ),
        )


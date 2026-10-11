package app

import (
	"fmt"
	"os"

	"github.com/Kaginari/agent-one/memory"
)

// cmdMCP is `<dist> mcp serve`: the workspace's memory as an MCP server on stdio, for Claude Code,
// OpenCode or any MCP client — recall, remember, memory_status. The workspace is the one found from
// the working directory up (or --root).
func cmdMCP(dist string, f cliFlags, args []string, io IO, v Version) int {
	if len(args) == 0 || args[0] != "serve" {
		fmt.Fprintf(io.Err, "usage: %s mcp serve [--root <dir>]  — the workspace's memory as an MCP server on stdio\n", dist)
		return 2
	}
	d, err := parseDist(dist)
	if err != nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? %v\n", err)
		return 2
	}
	memory.DefaultWorkspaceDir = d.WorkspaceDir
	root := f.root
	if root == "" {
		cwd, _ := os.Getwd()
		r, ok := findWorkspace(cwd, d.WorkspaceDir)
		if !ok {
			fmt.Fprintf(io.Err, "@S FAIL\n@? no %s here or above — found one with `%s init`, or pass --root\n", d.WorkspaceDir, dist)
			return 2
		}
		root = r
	}
	version := v.Version
	if version == "" {
		version = "dev"
	}
	if err := memory.ServeMCP(root, dist, version, io.In, io.Out); err != nil {
		fmt.Fprintf(io.Err, "@? mcp serve: %v\n", err)
		return 1
	}
	return 0
}

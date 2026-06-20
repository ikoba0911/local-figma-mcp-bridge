# Local Figma MCP Bridge

English | [日本語](./README.ja.md)

Unofficial local MCP bridge for reading the currently open Figma file through a Figma plugin.

This project is intended for local development and experimentation. It is not affiliated with, endorsed by, or sponsored by Figma.

```text
MCP client
  -> local MCP server (stdio)
  -> WebSocket bridge (localhost:8787)
  -> Figma Plugin UI
  -> Figma Plugin main thread
  -> currently open Figma file
```

## What It Does

- Checks whether the Figma plugin is connected to the local MCP server.
- Reads basic information about the open Figma file and current page.
- Summarizes the currently selected nodes, including structure, size, position, colors, layout metadata, and child nodes.
- Exports the current selection as `SVG` or `PNG`.

## Privacy And Data Handling

This tool can access design data from the Figma file where the plugin is running.

Data that may be sent from Figma to the local MCP server includes:

- File metadata such as the root file name, page name, page id, and Figma file key.
- Selected node metadata such as node ids, names, dimensions, positions, colors, effects, layout properties, and child hierarchy.
- Exported selection contents as base64-encoded `SVG` or `PNG`, which may include images, text, icons, product UI, customer data, or other confidential design material.

The default implementation sends this data only to a local WebSocket server at `ws://localhost:8787`. It does not send data to a remote service by itself, and the plugin manifest does not allow production network access to external domains.

Before using this with non-public design files, make sure you have permission from the file owner or organization. Do not publish exported design assets, customer work, internal UI, or file-derived sample data without explicit authorization.

## Security Notes

- The bridge is designed for local development. Do not expose `FIGMA_BRIDGE_HOST` or port `8787` to a public network.
- Treat any MCP client connected to this bridge as able to request selected Figma node metadata and exports.
- The MCP server does not currently authenticate WebSocket clients. Run it only on a trusted machine and keep the host bound to `localhost` unless you have added your own authentication and transport protections.
- Do not commit real exported Figma assets, `.env` files, access tokens, logs, or generated scratch output.

## Setup

```bash
npm install
npm run build
```

## Load The Figma Plugin

1. Open Figma Desktop.
2. Choose `Plugins > Development > Import plugin from manifest...`.
3. Select `packages/figma-plugin/dist/manifest.json`.
4. Run `Local Figma MCP Bridge`.
5. The plugin UI will connect automatically when the local MCP server is running.

If the UI shows `Disconnected`, start the local MCP server and wait for the plugin to reconnect:

```bash
npm run dev:mcp
```

## MCP Client Configuration

Any MCP client that supports stdio servers can use this bridge. Add the built MCP server to your MCP client settings. Codex configuration is shown below as one example:

```json
{
  "mcpServers": {
    "local-figma": {
      "command": "node",
      "args": [
        "/absolute/path/to/packages/mcp-server/dist/index.js"
      ],
      "env": {
        "FIGMA_BRIDGE_PORT": "8787"
      }
    }
  }
}
```

Optional environment variables:

- `FIGMA_BRIDGE_HOST`: WebSocket host. Defaults to `localhost`.
- `FIGMA_BRIDGE_PORT`: WebSocket port. Defaults to `8787`.

## MCP Tools

- `figma_status`: Check whether the Figma plugin is connected.
- `figma_file_info`: Get basic information about the open Figma file and current page.
- `figma_get_selection`: Return a JSON summary of the currently selected Figma nodes.
- `figma_export_selection`: Export the current Figma selection as base64-encoded `SVG` or `PNG`.

## Figma Manifest Network Access

The plugin manifest uses:

```json
{
  "networkAccess": {
    "allowedDomains": ["none"],
    "devAllowedDomains": [
      "http://localhost:8787",
      "ws://localhost:8787"
    ]
  }
}
```

This keeps production network access disabled while allowing the local development bridge. If you change this project to send data to remote services, update the manifest, documentation, privacy policy, and user consent flow accordingly.

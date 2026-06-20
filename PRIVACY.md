# Privacy Notice

This project does not send data to a remote service by default.

The Figma plugin connects to a local WebSocket bridge at `ws://localhost:8787`. When an MCP client calls a tool, the plugin may send the following data to the local MCP server:

- Figma file metadata, including root name, current page name, page id, selection count, and file key.
- Selected node metadata, including node ids, names, types, visibility, lock state, dimensions, positions, fills, strokes, effects, layout metadata, and child hierarchy.
- Exported selected nodes as base64-encoded `SVG` or `PNG`.

This data may include personal data, confidential product information, customer content, or third-party intellectual property depending on the Figma file you use.

Use this tool only with files you are authorized to inspect or export. If you modify this project to send data to any remote service, provide your own privacy policy, obtain any required permissions and consents, and update the Figma plugin manifest network access settings.

# Security Policy

This project is an unofficial local development bridge between Figma and MCP clients.

## Supported Use

Run the bridge only on a trusted local machine. The default MCP server listens on `localhost:8787` and does not authenticate WebSocket clients.

Do not expose the bridge port to a public or shared network unless you have added authentication, authorization, and transport protections.

## Sensitive Data

The bridge can transfer selected Figma node metadata and exported `SVG` or `PNG` data. Those exports may contain confidential product designs, customer content, images, text, icons, or other proprietary material.

Do not commit real Figma exports, customer assets, screenshots, logs, `.env` files, credentials, or generated scratch output.

## Reporting Issues

If you find a vulnerability, please report it privately to the repository maintainer instead of opening a public issue with exploit details.

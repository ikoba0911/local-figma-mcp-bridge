import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StdioServerTransport } from "@modelcontextprotocol/sdk/server/stdio.js";
import { z } from "zod";
import { WebSocket, WebSocketServer } from "ws";

type BridgeRequest = {
  id: string;
  type: "command";
  command: string;
  payload?: unknown;
};

type BridgeResponse = {
  id: string;
  type: "result" | "error";
  payload?: unknown;
  error?: string;
};

const port = Number(process.env.FIGMA_BRIDGE_PORT ?? 8787);
const host = process.env.FIGMA_BRIDGE_HOST ?? "localhost";
const pending = new Map<
  string,
  {
    resolve: (value: unknown) => void;
    reject: (error: Error) => void;
    timeout: NodeJS.Timeout;
  }
>();

let pluginSocket: WebSocket | undefined;
let lastPluginSeenAt: string | undefined;

const wss = new WebSocketServer({ host, port });

wss.on("connection", (socket) => {
  pluginSocket = socket;
  lastPluginSeenAt = new Date().toISOString();

  socket.on("message", (raw) => {
    let response: BridgeResponse;
    try {
      response = JSON.parse(raw.toString()) as BridgeResponse;
    } catch {
      return;
    }

    if (response.type === "result" || response.type === "error") {
      const entry = pending.get(response.id);
      if (!entry) return;

      clearTimeout(entry.timeout);
      pending.delete(response.id);

      if (response.type === "error") {
        entry.reject(new Error(response.error ?? "Figma plugin returned an error."));
      } else {
        entry.resolve(response.payload);
      }
    }
  });

  socket.on("close", () => {
    if (pluginSocket === socket) {
      pluginSocket = undefined;
    }
  });
});

function isPluginConnected() {
  return pluginSocket?.readyState === WebSocket.OPEN;
}

function sendToPlugin(command: string, payload?: unknown) {
  if (!isPluginConnected() || !pluginSocket) {
    throw new Error("Figma plugin is not connected. Run the Local Figma MCP Bridge plugin in Figma.");
  }

  const id = crypto.randomUUID();
  const request: BridgeRequest = { id, type: "command", command, payload };

  return new Promise<unknown>((resolve, reject) => {
    const timeout = setTimeout(() => {
      pending.delete(id);
      reject(new Error(`Timed out waiting for Figma plugin command: ${command}`));
    }, 20_000);

    pending.set(id, { resolve, reject, timeout });
    pluginSocket?.send(JSON.stringify(request));
  });
}

function asText(value: unknown) {
  return {
    content: [
      {
        type: "text" as const,
        text: typeof value === "string" ? value : JSON.stringify(value, null, 2)
      }
    ]
  };
}

const server = new McpServer({
  name: "local-figma-mcp-bridge",
  version: "0.1.0"
});

server.registerTool(
  "figma_status",
  {
    title: "Figma bridge status",
    description: "Check whether the local Figma plugin bridge is connected.",
    inputSchema: {}
  },
  async () =>
    asText({
      websocket: `ws://${host}:${port}`,
      connected: isPluginConnected(),
      lastPluginSeenAt
    })
);

server.registerTool(
  "figma_file_info",
  {
    title: "Figma file info",
    description: "Get basic information about the open Figma file and current page.",
    inputSchema: {}
  },
  async () => asText(await sendToPlugin("fileInfo"))
);

server.registerTool(
  "figma_get_selection",
  {
    title: "Get Figma selection",
    description: "Return a JSON summary of the currently selected Figma nodes.",
    inputSchema: {
      depth: z.number().int().min(0).max(5).default(2)
    }
  },
  async ({ depth }) => asText(await sendToPlugin("getSelection", { depth }))
);

server.registerTool(
  "figma_export_selection",
  {
    title: "Export Figma selection",
    description: "Export the current Figma selection as SVG or PNG. Result is base64 encoded.",
    inputSchema: {
      format: z.enum(["SVG", "PNG"]).default("SVG"),
      scale: z.number().min(0.1).max(4).default(1)
    }
  },
  async ({ format, scale }) => asText(await sendToPlugin("exportSelection", { format, scale }))
);

const transport = new StdioServerTransport();
await server.connect(transport);

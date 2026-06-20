type CommandMessage = {
  type: "mcp-command";
  requestId: string;
  command: string;
  payload?: unknown;
};

type SerializedNode = {
  id: string;
  name: string;
  type: string;
  visible?: boolean;
  locked?: boolean;
  x?: number;
  y?: number;
  width?: number;
  height?: number;
  fills?: unknown;
  strokes?: unknown;
  effects?: unknown;
  layoutMode?: string;
  primaryAxisSizingMode?: string;
  counterAxisSizingMode?: string;
  children?: SerializedNode[];
};

figma.showUI(__html__, { width: 320, height: 140, themeColors: true });

figma.ui.onmessage = async (message: CommandMessage) => {
  if (message.type !== "mcp-command") return;

  try {
    const payload = await runCommand(message.command, message.payload);
    figma.ui.postMessage({
      type: "mcp-result",
      requestId: message.requestId,
      payload
    });
  } catch (error) {
    figma.ui.postMessage({
      type: "mcp-error",
      requestId: message.requestId,
      error: error instanceof Error ? error.message : String(error)
    });
  }
};

async function runCommand(command: string, payload: unknown) {
  switch (command) {
    case "fileInfo":
      return getFileInfo();
    case "getSelection":
      return getSelection(payload);
    case "exportSelection":
      return exportSelection(payload);
    default:
      throw new Error(`Unknown command: ${command}`);
  }
}

function getFileInfo() {
  return {
    fileKey: figma.fileKey,
    rootName: figma.root.name,
    currentPage: {
      id: figma.currentPage.id,
      name: figma.currentPage.name,
      selectionCount: figma.currentPage.selection.length
    }
  };
}

function getSelection(payload: unknown) {
  const depth = readNumber(payload, "depth", 2);
  const fileInfo = getFileInfo();

  return {
    fileKey: fileInfo.fileKey,
    rootName: fileInfo.rootName,
    currentPage: fileInfo.currentPage,
    selection: figma.currentPage.selection.map((node) => serializeNode(node, depth))
  };
}

async function exportSelection(payload: unknown) {
  const selection = figma.currentPage.selection;
  if (selection.length === 0) {
    throw new Error("Select at least one node in Figma before exporting.");
  }

  const format = readString(payload, "format", "SVG").toUpperCase();
  const scale = readNumber(payload, "scale", 1);

  if (format !== "SVG" && format !== "PNG") {
    throw new Error("format must be SVG or PNG.");
  }

  const exports = await Promise.all(
    selection.map(async (node) => {
      const bytes = await node.exportAsync(
        format === "SVG"
          ? { format: "SVG" }
          : {
              format: "PNG",
              constraint: {
                type: "SCALE",
                value: scale
              }
            }
      );

      return {
        id: node.id,
        name: node.name,
        format,
        mimeType: format === "SVG" ? "image/svg+xml" : "image/png",
        base64: bytesToBase64(bytes)
      };
    })
  );

  return { exports };
}

function serializeNode(node: SceneNode, depth: number): SerializedNode {
  const serialized: SerializedNode = {
    id: node.id,
    name: node.name,
    type: node.type,
    visible: "visible" in node ? node.visible : undefined,
    locked: "locked" in node ? node.locked : undefined
  };

  if ("x" in node) serialized.x = round(node.x);
  if ("y" in node) serialized.y = round(node.y);
  if ("width" in node) serialized.width = round(node.width);
  if ("height" in node) serialized.height = round(node.height);
  if ("fills" in node) serialized.fills = serializePaints(node.fills);
  if ("strokes" in node) serialized.strokes = serializePaints(node.strokes);
  if ("effects" in node) serialized.effects = node.effects;
  if ("layoutMode" in node) serialized.layoutMode = node.layoutMode;
  if ("primaryAxisSizingMode" in node) serialized.primaryAxisSizingMode = node.primaryAxisSizingMode;
  if ("counterAxisSizingMode" in node) serialized.counterAxisSizingMode = node.counterAxisSizingMode;

  if (depth > 0 && "children" in node) {
    serialized.children = node.children.map((child) => serializeNode(child, depth - 1));
  }

  return serialized;
}

function serializePaints(paints: readonly Paint[] | PluginAPI["mixed"]) {
  if (paints === figma.mixed) return "mixed";

  return paints.map((paint) => {
    if (paint.type === "SOLID") {
      return {
        type: paint.type,
        visible: paint.visible,
        opacity: paint.opacity,
        color: {
          r: round(paint.color.r * 255),
          g: round(paint.color.g * 255),
          b: round(paint.color.b * 255)
        }
      };
    }

    return {
      type: paint.type,
      visible: paint.visible,
      opacity: paint.opacity
    };
  });
}

function readNumber(payload: unknown, key: string, fallback: number) {
  if (!payload || typeof payload !== "object") return fallback;
  const value = (payload as Record<string, unknown>)[key];
  return typeof value === "number" ? value : fallback;
}

function readString(payload: unknown, key: string, fallback: string) {
  if (!payload || typeof payload !== "object") return fallback;
  const value = (payload as Record<string, unknown>)[key];
  return typeof value === "string" ? value : fallback;
}

function bytesToBase64(bytes: Uint8Array) {
  let binary = "";
  bytes.forEach((byte) => {
    binary += String.fromCharCode(byte);
  });
  return btoa(binary);
}

function round(value: number) {
  return Math.round(value * 100) / 100;
}

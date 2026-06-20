type BridgeRequest = {
  id: string;
  type: "command";
  command: string;
  payload?: unknown;
};

type MainResponse = {
  type: "mcp-result" | "mcp-error";
  requestId: string;
  payload?: unknown;
  error?: string;
};

const bridgeUrl = "ws://localhost:8787";
const statusEl = document.getElementById("status")!;
const dotEl = document.getElementById("dot")!;
const urlEl = document.getElementById("url")!;
const reconnectButton = document.getElementById("reconnect")!;

let socket: WebSocket | undefined;
let reconnectTimer: number | undefined;
let reconnectDelayMs = 500;
const pendingRequests = new Map<string, BridgeRequest>();

urlEl.textContent = bridgeUrl;

function setConnected(connected: boolean) {
  statusEl.textContent = connected ? "Connected" : "Disconnected";
  dotEl.classList.toggle("connected", connected);
}

function scheduleReconnect() {
  if (reconnectTimer !== undefined) return;

  reconnectTimer = window.setTimeout(() => {
    reconnectTimer = undefined;
    connect();
  }, reconnectDelayMs);

  reconnectDelayMs = Math.min(reconnectDelayMs * 2, 5000);
}

function clearReconnectTimer() {
  if (reconnectTimer === undefined) return;

  window.clearTimeout(reconnectTimer);
  reconnectTimer = undefined;
}

function connect() {
  clearReconnectTimer();

  socket?.close();
  const currentSocket = new WebSocket(bridgeUrl);
  socket = currentSocket;

  currentSocket.addEventListener("open", () => {
    if (socket !== currentSocket) return;

    reconnectDelayMs = 500;
    setConnected(true);
  });

  currentSocket.addEventListener("close", () => {
    if (socket !== currentSocket) return;

    setConnected(false);
    scheduleReconnect();
  });

  currentSocket.addEventListener("error", () => {
    if (socket !== currentSocket) return;

    setConnected(false);
  });

  currentSocket.addEventListener("message", (event) => {
    const request = JSON.parse(event.data) as BridgeRequest;
    if (request.type !== "command") return;

    pendingRequests.set(request.id, request);
    parent.postMessage(
      {
        pluginMessage: {
          type: "mcp-command",
          requestId: request.id,
          command: request.command,
          payload: request.payload
        }
      },
      "*"
    );
  });
}

window.onmessage = (event) => {
  const message = event.data.pluginMessage as MainResponse | undefined;
  if (!message || !("requestId" in message)) return;

  const request = pendingRequests.get(message.requestId);
  if (!request) return;

  pendingRequests.delete(message.requestId);

  if (socket?.readyState !== WebSocket.OPEN) return;

  socket.send(
    JSON.stringify({
      id: message.requestId,
      type: message.type === "mcp-error" ? "error" : "result",
      payload: message.payload,
      error: message.error
    })
  );
};

reconnectButton.addEventListener("click", connect);
connect();

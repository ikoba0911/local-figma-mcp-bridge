"use strict";
(() => {
  // src/ui.ts
  var bridgeUrl = "ws://localhost:8787";
  var statusEl = document.getElementById("status");
  var dotEl = document.getElementById("dot");
  var urlEl = document.getElementById("url");
  var reconnectButton = document.getElementById("reconnect");
  var socket;
  var pendingRequests = /* @__PURE__ */ new Map();
  urlEl.textContent = bridgeUrl;
  function setConnected(connected) {
    statusEl.textContent = connected ? "Connected" : "Disconnected";
    dotEl.classList.toggle("connected", connected);
  }
  function connect() {
    socket == null ? void 0 : socket.close();
    socket = new WebSocket(bridgeUrl);
    socket.addEventListener("open", () => setConnected(true));
    socket.addEventListener("close", () => setConnected(false));
    socket.addEventListener("error", () => setConnected(false));
    socket.addEventListener("message", (event) => {
      const request = JSON.parse(event.data);
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
    const message = event.data.pluginMessage;
    if (!message || !("requestId" in message)) return;
    const request = pendingRequests.get(message.requestId);
    if (!request) return;
    pendingRequests.delete(message.requestId);
    if ((socket == null ? void 0 : socket.readyState) !== WebSocket.OPEN) return;
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
})();

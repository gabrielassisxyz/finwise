(function () {
  'use strict';

  function createSSEClient(url, options) {
    options = options || {};
    const maxBackoff = options.maxBackoff || 30000;
    const initialBackoff = options.initialBackoff || 1000;
    const heartbeatTimeout = options.heartbeatTimeout || 10000;

    let es = null;
    let lastEventId = null;
    let reconnectTimer = null;
    let heartbeatTimer = null;
    let currentBackoff = initialBackoff;
    let isClosed = false;
    let onMessage = options.onMessage || function () {};
    let onError = options.onError || function () {};
    let onOpen = options.onOpen || function () {};

    function clearTimers() {
      if (reconnectTimer) {
        clearTimeout(reconnectTimer);
        reconnectTimer = null;
      }
      if (heartbeatTimer) {
        clearTimeout(heartbeatTimer);
        heartbeatTimer = null;
      }
    }

    function resetHeartbeat() {
      if (heartbeatTimer) clearTimeout(heartbeatTimer);
      heartbeatTimer = setTimeout(function () {
        if (es) {
          es.close();
          scheduleReconnect();
        }
      }, heartbeatTimeout);
    }

    function scheduleReconnect() {
      if (isClosed) return;
      clearTimers();
      reconnectTimer = setTimeout(function () {
        connect();
      }, currentBackoff);
      currentBackoff = Math.min(currentBackoff * 2, maxBackoff);
    }

    function connect() {
      if (isClosed) return;
      clearTimers();

      let connectUrl = url;
      if (lastEventId) {
        const sep = url.indexOf('?') === -1 ? '?' : '&';
        connectUrl = url + sep + 'lastEventId=' + encodeURIComponent(lastEventId);
      }

      try {
        es = new EventSource(connectUrl);
      } catch (e) {
        onError(e);
        scheduleReconnect();
        return;
      }

      es.onopen = function () {
        currentBackoff = initialBackoff;
        resetHeartbeat();
        onOpen();
      };

      es.onmessage = function (event) {
        resetHeartbeat();
        if (event.lastEventId) {
          lastEventId = event.lastEventId;
        }
        if (event.data === '' || event.data === 'heartbeat' || event.data === 'ping') {
          return;
        }
        onMessage(event);
      };

      es.onerror = function (event) {
        clearTimers();
        onError(event);
        if (es) {
          es.close();
          es = null;
        }
        scheduleReconnect();
      };
    }

    function close() {
      isClosed = true;
      clearTimers();
      if (es) {
        es.close();
        es = null;
      }
    }

    connect();

    return {
      close: close,
      getLastEventId: function () { return lastEventId; }
    };
  }

  window.FinWiseSSE = {
    connect: createSSEClient
  };
})();

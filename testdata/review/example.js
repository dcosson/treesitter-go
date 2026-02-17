class EventEmitter {
  constructor() {
    this.listeners = new Map();
  }

  on(event, callback) {
    if (!this.listeners.has(event)) {
      this.listeners.set(event, []);
    }
    this.listeners.get(event).push(callback);
    return this;
  }

  emit(event, ...args) {
    const callbacks = this.listeners.get(event) || [];
    for (const cb of callbacks) {
      cb(...args);
    }
  }

  off(event, callback) {
    const callbacks = this.listeners.get(event);
    if (callbacks) {
      const idx = callbacks.indexOf(callback);
      if (idx !== -1) {
        callbacks.splice(idx, 1);
      }
    }
  }
}

async function fetchData(url) {
  try {
    const response = await fetch(url);
    if (!response.ok) {
      throw new Error(`HTTP ${response.status}`);
    }
    const data = await response.json();
    return data;
  } catch (error) {
    console.error("Fetch failed:", error.message);
    return null;
  }
}

const emitter = new EventEmitter();
emitter.on("data", (item) => {
  console.log(`Received: ${JSON.stringify(item)}`);
});

fetchData("https://api.example.com/items")
  .then((data) => {
    if (data) {
      emitter.emit("data", data);
    }
  });

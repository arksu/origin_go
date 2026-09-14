# Initialize the game renderer before opening the WebSocket

`GameView` currently connects first and creates its canvas only after the connection reaches `connected`. Server `objectSpawn` messages may therefore reach `ObjectView` while Pixi initialization is still awaiting `Application.init()`, before `ActorRenderer` is assigned. This throws for 3D characters. Messages arriving even earlier are silently dropped by `GameFacade`.

Keep the canvas wrapper mounted across connection states and hide it until connected. On mount, initialize the canvas and await `GameFacade.init()` before opening the WebSocket. Retrying a failed connection reuses the initialized canvas. If renderer initialization fails, show an error and do not connect. On unmount, disconnect before destroying the renderer. No packet buffering is needed because no world packets can arrive before the renderer is ready.

Validate with the client type check and build. Verify that connection is called only after successful renderer initialization, and that the canvas DOM node survives connection-state changes.

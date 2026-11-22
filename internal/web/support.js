{
  const { desk, inboxes } = self.boreasSupport;
  const inboxID = inboxes[location.pathname.split("/")[1]] ?? inboxes["*"];
  if (inboxID) {
    self.LibredeskSettings = { baseURL: desk, inboxID };
    addEventListener("load", () =>
      document.body.append(Object.assign(document.createElement("script"), { src: `${desk}/widget.js`, fetchPriority: "low" }))
    );
  }
}

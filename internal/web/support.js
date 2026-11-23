if (location.pathname.split("/").filter(Boolean).length < 2)
  addEventListener("load", () =>
    document.body.append(Object.assign(document.createElement("script"), { src: `${LibredeskSettings.baseURL}/widget.js`, fetchPriority: "low" }))
  );

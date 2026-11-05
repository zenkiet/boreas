{
  const lock = document.querySelector(".lock");
  const bora = lock.querySelector(".bora");
  const input = document.getElementById("password");
  const show = lock.querySelector(".show");
  const unlock = lock.querySelector(".unlock");

  setTimeout(() => lock.classList.add("ready"), lock.classList.contains("wrong") ? 900 : 450);
  input.addEventListener("input", () => lock.classList.remove("wrong", "wait"));

  show.hidden = false;
  show.addEventListener("mousedown", (event) => event.preventDefault());
  show.addEventListener("click", () => {
    const visible = input.type === "password";
    input.type = visible ? "text" : "password";
    show.textContent = visible ? "Hide" : "Show";
    show.setAttribute("aria-pressed", visible);
    lock.classList.toggle("peek", visible);
  });

  input.form.addEventListener("submit", () => {
    lock.classList.add("thinking");
    unlock.disabled = true;
    unlock.textContent = "Unlocking…";
  });

  if (
    matchMedia("(hover: hover) and (pointer: fine) and (prefers-reduced-motion: no-preference)")
      .matches
  ) {
    const reach = (offset) => Math.max(-1, Math.min(1, offset / 160));
    let last;
    let frame = 0;
    addEventListener("pointermove", (event) => {
      last = event;
      frame ||= requestAnimationFrame(() => {
        frame = 0;
        const box = bora.getBoundingClientRect();
        bora.style.setProperty("--lx", reach(last.clientX - box.left - box.width / 2) * 2.4);
        bora.style.setProperty("--ly", reach(last.clientY - box.top - box.height / 2) * 2);
      });
    });
  }
}

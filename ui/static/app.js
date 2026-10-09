// On a phone the sidebar becomes a horizontally scrolling tab strip; keep the
// current section in view instead of leaving it past the right edge.
(() => {
  const active = document.querySelector(".sidebar nav a.active");
  const nav = active?.parentElement;
  if (!nav || nav.scrollWidth <= nav.clientWidth) return;
  const offset = active.getBoundingClientRect().left - nav.getBoundingClientRect().left;
  nav.scrollLeft += offset - (nav.clientWidth - active.offsetWidth) / 2;
})();

// A narrow screen scrolls the revenue chart; start at the recent months.
for (const chart of document.querySelectorAll(".chart")) {
  chart.scrollLeft = chart.scrollWidth;
}

(() => {
  const editor = document.querySelector("[data-item-editor]");
  if (!editor) return;

  const rows = editor.querySelector("[data-item-rows]");
  const template = editor.querySelector("[data-item-row-template]");

  editor.querySelector("[data-add-item]").addEventListener("click", () => {
    rows.append(template.content.cloneNode(true));
    rows.lastElementChild.querySelector('input[name="item_description"]').focus();
  });

  rows.addEventListener("click", (event) => {
    const button = event.target.closest("[data-remove-item]");
    if (button) button.closest("[data-item-row]").remove();
  });
})();

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

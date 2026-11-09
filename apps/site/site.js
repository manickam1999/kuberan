// The only script on the page: a progressive "Copy" button for the install
// commands. Without JavaScript the button stays hidden and the commands are
// still selectable with one click (user-select: all).
(function () {
  var button = document.querySelector("button.copy[data-copy]");
  if (!button || !navigator.clipboard) return;
  button.hidden = false;
  button.addEventListener("click", function () {
    navigator.clipboard.writeText(button.getAttribute("data-copy")).then(
      function () {
        button.textContent = "Copied";
        setTimeout(function () {
          button.textContent = "Copy";
        }, 1800);
      },
      function () {
        button.textContent = "Press Ctrl+C";
      }
    );
  });
})();

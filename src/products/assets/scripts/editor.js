DOM.Ready(function() {
  addEventListener("trix-initialize", function(e) {
    const file_tools = document.querySelector(".trix-button-group--file-tools");
    file_tools.remove();
  })
  addEventListener("trix-file-accept", function(e) {
    e.preventDefault();
  })
})

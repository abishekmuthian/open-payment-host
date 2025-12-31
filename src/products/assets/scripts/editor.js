DOM.Ready(function () {
  // Don't check for window.trix here - it may not be loaded yet
  // The event listener will only fire if Trix is present on the page

  // Disable File Attachments
  /*   addEventListener("trix-initialize", function (e) {
    const file_tools = document.querySelector(".trix-button-group--file-tools");
    file_tools.remove();
  });
  addEventListener("trix-file-accept", function (e) {
    e.preventDefault();
  }); */

  // Handle File Attachments
  document.addEventListener("trix-attachment-add", function (event) {
    if (event.attachment.file) {
      uploadFile(event.attachment);
    }
  });

  function uploadFile(attachment) {
    let formData = new FormData();
    formData.append("file", attachment.file);
    formData.append("authenticity_token", authenticityToken());

    fetch("/product/editor/upload", {
      method: "POST",
      body: formData,
    })
      .then((response) => response.json())
      .then((data) => {
        attachment.setAttributes({
          url: data.url,
          href: data.url,
        });
      })
      .catch((error) => {
        console.error("Error uploading file:", error);
      });
  }

  // Editor suggestions
  let isAutocompleteActive = false;

  function triggerAutocomplete(trixEditorElement) {
    const trixEditor = trixEditorElement.editor;
    const currentText = trixEditor.getDocument().toString().trim();
    const originalLength = currentText.length;

    let formData = new FormData();
    formData.append("text", currentText);
    formData.append("authenticity_token", authenticityToken());

    fetch("/product/editor/suggestion", {
      method: "POST",
      body: formData,
    })
      .then((response) => response.json())
      .then((data) => {
        if (data.completion) {
          trixEditor.insertHTML(data.completion);
          const newLength = trixEditor.getDocument().toString().trim().length;
          const insertedLength = newLength - originalLength;

          const startPosition = originalLength;
          const endPosition = startPosition + insertedLength;

          trixEditor.setSelectedRange([startPosition, endPosition]);
          isAutocompleteActive = true;
        }
      });
  }

  const trixEditorElement = document.querySelector("trix-editor");
  if (trixEditorElement) {
    trixEditorElement.addEventListener("keydown", function (event) {
      const trixEditor = this.editor;

      if (event.key === "Tab" && isAutocompleteActive) {
        event.preventDefault(); // prevent default Tab behavior
        const endOfSelection = trixEditor.getSelectedRange()[1];
        trixEditor.setSelectedRange([endOfSelection, endOfSelection]); // move cursor to end of selection
        isAutocompleteActive = false; // reset the autocomplete flag
      }

      // Trigger autocomplete on a period followed by space
      const currentText = trixEditor.getDocument().toString().trim();
      if (event.key === " " && currentText.slice(-1) === ".") {
        triggerAutocomplete(this);
      }
    });
  } else {
    console.log("Not loading Trix editor on this page");
  }
});

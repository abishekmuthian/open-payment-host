/* import { Editor } from 'https://esm.sh/@tiptap/core'
import StarterKit from 'https://esm.sh/@tiptap/starter-kit'
import Text from 'https://esm.sh/@tiptap/extension-text'

DOM.Ready(function() {
  console.log("Called in editor")
  new Editor({
    element: document.querySelector('.element'),
    editorProps: {
      attributes: {
        class: 'prose prose-sm sm:prose lg:prose-lg xl:prose-2xl mx-auto focus:outline-none',
      },
    },
    extensions: [
      StarterKit,
    ],
    editable: true,
    injectCSS: false,
    injectNonce: nonceToken(),
    content: '<p>Hello World!</p>',
  })
})


// nonceToken returns the nonce token from the page
function nonceToken() {
  // Collect the authenticity token from meta tags in header
  var meta = DOM.First("meta[name='nonce_token']")
  if (meta === undefined) {
      e.preventDefault();
      return ""
  }
  return meta.getAttribute('content');
} */
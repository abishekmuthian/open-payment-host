/* JS for products */

DOM.Ready(function() {
   // Count characters in Title
   var el = document.getElementById('title');
   if(el){
     el.addEventListener('input', SetTitleCharacterCount);
   }

  // Watch story form to fetch title of story page
  SetSubmitStoryName();
});

// Set live character count in Title
function SetTitleCharacterCount(){
  var maxLimit = 50;
  var lengthCount = RemoveHashtag(this.value).length;
  // Replacing the content automatically, using maxlength attribute instead
  // Does not work properly with editable so not having it in text.
 /* if (lengthCount > maxLimit) {
      this.value = this.value.substring(0, maxLimit);
      var charactersLeft = maxLimit - lengthCount + 1;                   
  }
  else {                   
      var charactersLeft = maxLimit - lengthCount;                   
  }*/
  // Initially set the charactercount not to display
  document.getElementById('spnTitleCharLeft').style.display = 'none';

  if(lengthCount > maxLimit){
    //var charactersLeft = maxLimit - lengthCount;  
    document.getElementById('spnTitleCharLeft').style.display = '';                                 
    document.getElementById('spnTitleCharLeft').innerHTML="Character limit reached: "+lengthCount;
  }

  if(CountHashtag(this.value)>2){
    document.getElementById('spnTitleCharLeft').style.display = '';
    document.getElementById('spnTitleCharLeft').innerHTML="Hashtag limit reached: "+CountHashtag(this.value);
  }

}

// Remove Hashtags from text
function RemoveHashtag(text){
  var regexp = new RegExp('#([^\\s]*)','g');
  return text.replace(regexp, '');
}

// Count Hashtags in the text
function CountHashtag(text){
  var regexp = new RegExp('#([^\\s]*)','g');
  if ( text.match(regexp) == null){
    return 0
  }else{
    return text.match(regexp).length;
  }
}

// SetSubmitStoryName sets the story name from a URL
// Attempt to extract a last param from URL, and set name to a munged version of that
function SetSubmitStoryName() {
  DOM.On('.active_url_field', 'change', function(e) {
    var name = DOM.First('.name_field')
    var summary = DOM.First('.summary_field')
    var original_name = name.value
    var original_url = this.value
    
    // First locally fill in the name field 
    if (original_name== "") {
      
      // For github urls, try to fetch some more info 
      if (original_url.startsWith('https://github.com/abishekmuthian/open-payment-host/')) {
      
        var url = original_url.replace('https://github.com/abishekmuthian/open-payment-host/','https://api.github.com/abishekmuthian/open-payment-host/repos/')
         DOM.Get(url,function(request){
           var data = JSON.parse(request.response);
          
           // if we got a reponse, try using it. 
           name.value =  data.name + " - " + data.description
           summary.value = data.description + " by " + data.owner.login
           
           // later use 
           // created_at -> original_published_at
           // updated_at -> original_updated_at
           // data.owner.name -> original_author
           
         },function(){
           console.log("failed to fetch github data")
         });
      
        return false;
      } 
      
      // We could also attempt to fetch the html page, and grab metadata from it 
      // author, pubdate, metadesc etc
      // would this be better done in a background way after story submission?
      
      
      // Else just use name from local url if we can
      name.value = urlToSentenceCase(original_url);
    
  
    }
  
    
    
  });

}

// Change a URL to a sentence for SetSubmitStoryName
function urlToSentenceCase(url) {
  if (url === undefined) {
    return ""
  }
  
  var parts, name
  url = url.replace(/\/$/, ""); // remove trailing /
  parts = url.split("/"); // now split on /
  name = parts[parts.length - 1]; // last part of string after last /
  name = name.replace(/[\?#].*$/, ""); //remove anything after ? or #
  name = name.replace(/^\d*-/, ""); // remove prefix numerals with dash (common on id based keys)
  name = name.replace(/\..*$/, ""); // remove .html etc extensions
  name = name.replace(/[_\-+]/g, " "); // remove all - or + or _ in string, replacing with space
  name = name.trim(); // remove whitespace trailing or leading
  name = name.toLowerCase(); // all lower
  name = name[0].toUpperCase() + name.substring(1); // Sentence case
  
  
  // Deal with some specific URLs
  if (url.match(/youtube|vimeo\.com/)) {
     name = "Video: "
  }
  if (url.match(/medium\.com/)) {
      // Eat the last word (UDID) on medium posts
      name = name.replace(/ [^ ]*$/, "");
  }

  
  
  return name
}
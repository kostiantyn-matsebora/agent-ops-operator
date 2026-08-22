// The player, for a page that publishes a recording.
//
// The page writes an ordinary markdown link — the recording as the target, the
// poster image with its alt text as the content — and names `{: .ao-demo}` on
// it. Both files are the page's, both words are the page's, and this supplies
// the element and its controls.
//
// WITH NO SCRIPTING IT IS A POSTER LINKING TO THE FILE, which is a working panel
// rather than a fallback for one: the reader sees the product and can open the
// recording. That is the same bargain the tab strip makes.
//
// `preload="none"` is what keeps the megabytes off a visitor who never presses
// play — the poster is the only thing the page fetches.
(function () {
  var content = document.getElementById('ao-content');
  if (!content) return;

  [].forEach.call(content.querySelectorAll('a.ao-demo'), function (link) {
    var poster = link.querySelector('img');
    var src = link.getAttribute('href');
    if (!poster || !src) return;

    var video = document.createElement('video');
    video.className = 'ao-demo';
    video.controls = true;
    video.preload = 'none';
    video.setAttribute('playsinline', '');
    video.setAttribute('poster', poster.getAttribute('src'));
    video.setAttribute('src', src);

    // A video element has no alt text, so the poster's becomes its name. The
    // words are the page's either way.
    var label = poster.getAttribute('alt');
    if (label) video.setAttribute('aria-label', label);

    // The caption track, when the page names one. It is what a silent recording
    // has instead of narration: TEXT, in its own file, so it can be translated,
    // read aloud and turned off. Burned-in words could be none of those.
    var captions = link.getAttribute('data-captions');
    if (captions) {
      var track = document.createElement('track');
      track.kind = 'captions';
      track.label = 'English';
      track.srclang = 'en';
      // The attribute, not the property: the property is read after loading,
      // and by then the browser has already decided to show nothing.
      track.setAttribute('default', '');
      track.src = captions;
      video.appendChild(track);
    }

    link.parentNode.replaceChild(video, link);

    if (window.agentops && window.agentops.themed) {
      window.agentops.themed(video, 'src');
      window.agentops.themed(video, 'poster');
    }
  });
})();

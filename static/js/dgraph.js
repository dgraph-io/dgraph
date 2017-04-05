// debounce limits the amount of function invocation by spacing out the calls
// by at least `wait` ms.
function debounce(func, wait, immediate) {
  var timeout;

  return function() {
    var context = this, args = arguments;
    var later = function() {
      timeout = null;
      if (!immediate) func.apply(context, args);
    };

    var callNow = immediate && !timeout;
    clearTimeout(timeout);
    timeout = setTimeout(later, wait);
    if (callNow) func.apply(context, args);
  };
};

function slugify(text) {
  return text.toString().toLowerCase()
    .replace(/\s+/g, '-')           // Replace spaces with -
    .replace(/[^\w\-]+/g, '')       // Remove all non-word chars
    .replace(/\-\-+/g, '-')         // Replace multiple - with single -
    .replace(/^-+/, '')             // Trim - from start of text
    .replace(/-+$/, '');            // Trim - from end of text
}

// isElementInViewport checks if element is visible in the DOM
function isElementInViewport(el) {
  var rect = el.getBoundingClientRect();
  var topbarOffset = 64;

  return (
    rect.top >= topbarOffset &&
    rect.left >= 0 &&
    rect.bottom <= (window.innerHeight || document.documentElement.clientHeight) &&
    rect.right <= (window.innerWidth || document.documentElement.clientWidth)
  );
}

(function() {
  // clipboard
  var clipInit = false;
  $("pre code:not(.no-copy)").each(function() {
    var code = $(this), text = code.text();

    if (text.length > 5) {
      if (!clipInit) {
        var text;
        var clip = new Clipboard(".copy-btn", {
          text: function(trigger) {
            text = $(trigger).prev("code").text();
            return text.replace(/^\$\s/gm, "");
          }
        });

        clip.on("success", function(e) {
          e.clearSelection();
          $(e.trigger).text("Copied to clipboard!")
            .addClass('copied');

          window.setTimeout(function() {
            $(e.trigger).text("Copy").removeClass('copied');
          }, 2000);
        });

        clip.on("error", function(e) {
          e.clearSelection();
          $(e.trigger).text("Error copying");

          window.setTimeout(function() {
            $(e.trigger).text("Copy");
          }, 2000);
        });

        clipInit = true;
      }

      code.after('<span class="copy-btn">Copy</span>');
    }
  });

  // Sidebar
  var h2s = document.querySelectorAll("h2");
  var h3s = document.querySelectorAll("h3");
  var isAfter = function(e1, e2) {
    return e1.compareDocumentPosition(e2) & Node.DOCUMENT_POSITION_FOLLOWING;
  };
  var activeLink = document.querySelector(".topic.active");
  var allLinks = [];

  var h2sWithH3s = [];
  var j = 0;
  for (var i = 0; i < h2s.length; i++) {
    var h2 = h2s[i];
    var nextH2 = h2s[i + 1];
    var ourH3s = [];
    while (
      h3s[j] && isAfter(h2, h3s[j]) && (!nextH2 || !isAfter(nextH2, h3s[j]))
    ) {
      ourH3s.push({ header: h3s[j] });
      j++;
    }

    h2sWithH3s.push({
      header: h2,
      subHeaders: ourH3s
    });
  }

  // console.log(h2sWithH3s);

  function createSubtopic(container, headers) {
    var subMenu = document.createElement("ul");
    subMenu.className = "sub-topics";
    container.appendChild(subMenu);

    Array.prototype.forEach.call(headers, function(h) {
      var li = createSubtopicItem(h.header);
      li.className = 'topic sub-topic';
      subMenu.appendChild(li);

      if (h.subHeaders) {
        createSubtopic(subMenu, h.subHeaders)
      }
    });
  }

  function createSubtopicItem(h) {
  allLinks.push(h);

    var li = document.createElement("li");
    li.innerHTML = '<i class="fa fa-angle-right"></i> <a href="#' +
      h.id +
      '" data-scroll class="' +
      h.tagName +
      '">' +
      (h.title || h.textContent) +
      "</a>";
    return li;
  }

  // setActiveSubTopic updates the active subtopic on the sidebar based on the
  // hash
  // @params hash [String] - hash including the hash sign at the beginning
  function setActiveSubTopic(hash) {
    // Set inactive the previously active topic
    var prevActiveTopic = document.querySelector('.sub-topics .topic.active');
    var nextActiveTopic = document.querySelector('.sub-topics a[href="' + hash + '"]').parentNode;

    if (prevActiveTopic !== nextActiveTopic) {
      nextActiveTopic.classList.add('active');

      if (prevActiveTopic) {
        prevActiveTopic.classList.remove('active');
      }
    }
  }

  // updateSidebar updates the active menu in the sidebar
  function updateSidebar() {
    var currentScrollY = document.body.scrollTop;

    var activeHash;
    for (var i = 0; i < allLinks.length; i++) {
      var h = allLinks[i];
      var hash = h.getElementsByTagName('a')[0].hash;

      if (h.offsetTop - 250 > currentScrollY) {
        if (!activeHash) {
          activeHash = hash;
          break;
        }
      } else {
        activeHash = hash;
      }
    }

    if (activeHash) {
      setActiveSubTopic(activeHash);
    }
  }

  if (h2sWithH3s.length > 0) {
    createSubtopic(activeLink, h2sWithH3s);
  }

  var subTopics = document.querySelectorAll('.sub-topics .sub-topic')
  for (var i = 0; i < subTopics.length; i++) {
    var subTopic = subTopics[i]
    subTopic.addEventListener('click', function (e) {
      var hash = e.target.hash;
      setActiveSubTopic(hash);
    });
  }

  // Scrollspy for sidebar
  window.addEventListener('scroll', debounce(updateSidebar, 15));

  // Sidebar toggle
  document.getElementById('sidebar-toggle').addEventListener('click', function (e) {
    e.preventDefault();
    var klass = document.body.className;
    if (klass === "sidebar-visible") {
      document.body.className = "";
    } else {
      document.body.className = "sidebar-visible";
    }
  });

  // Anchor tags for headings
  function appendAnchor(heading) {
    // First remove the id from heading
    // Instead we will assign the id to the .anchor-offset element to account
    // for the fixed header height
    heading.id = '';

    var text = heading.innerText;
    var slug = slugify(text);

    var anchorOffset = document.createElement('div');
    anchorOffset.className = 'anchor-offset';
    anchorOffset.id = slug;

    var anchor = document.createElement("a");
    anchor.href = '#' + slug;
    anchor.className = 'anchor';
    // anchor.innerHTML = 'link'
    anchor.innerHTML = '<i class="fa fa-link"></i>'
    heading.insertBefore(anchor, heading.firstChild);
    heading.insertBefore(anchorOffset, heading.firstChild);
  }
  var h2s = document.querySelectorAll(
    '.content-wrapper h2, .content-wrapper h3');
  for (var i = 0; i < h2s.length; i++) {
    appendAnchor(h2s[i]);
  }

  // code collapse
  var pres = document.getElementsByTagName("pre");
  Array.prototype.forEach.call(pres, function(pre) {
    if (pre.clientHeight > 330) {
      pre.className += " collapsed";

      var showMore = document.createElement("div");
      showMore.className = "showmore";
      showMore.innerHTML = "<span>Show all</span>";
      showMore.addEventListener("click", function() {
        pre.className = "";
        showMore.parentNode.removeChild(showMore);
      });

      pre.appendChild(showMore);
    }
  });

  // version selector
  var currentVersion = location.pathname.split("/")[1];
  document
    .getElementsByClassName("version-selector")[0]
    .addEventListener("change", function(e) {
      var targetVersion = e.target.value;

      if (currentVersion !== targetVersion) {
        // Getting everything after targetVersion and concatenating it with the hash part.
        var targetPath = "/" +
          targetVersion +
          "/" +
          location.pathname.split("/").slice(2).join("/") +
          location.hash;
        location.assign(targetPath);
      }
    });

  var versionSelector = document.getElementsByClassName("version-selector")[0],
    options = versionSelector.options;

  for (var i = 0; i < options.length; i++) {
    if (options[i].value.indexOf("latest") != -1) {
      options[i].value = options[i].value.replace(/\s\(latest\)/, "");
    }
  }

  for (var i = 0; i < options.length; i++) {
    if (options[i].value === currentVersion) {
      options[i].selected = true;
      break;
    }
  }

  // Add target = _blank to all external links.
  var links = document.links;

  for (var i = 0, linksLength = links.length; i < linksLength; i++) {
    if (links[i].hostname != window.location.hostname) {
      links[i].target = "_blank";
    }
  }

  // Runnables
  $('.runnable').each(function () {
    var el = this;
    var codeEl = $(el).find('.output')[0];

    // Running code
    $(el).find('[data-action="run"]').on('click', function (e) {
      e.preventDefault();
      var query = $(el).find('.query-editable').text();

      $(el).find('.output-container').removeClass('empty error');
      codeEl.innerText = 'Waiting for the server response...';

      $.post('https://play.dgraph.io/query', query)
        .done(function (res) {
        var resText = JSON.stringify(res, null, 2);

        codeEl.innerText = resText;
        hljs.highlightBlock(codeEl);
      })
      .fail(function (xhr, status, error) {
        $(el).find('.output-container').addClass('error');

        codeEl.innerText = xhr.responseText || error;
      });
    });

    // Refresh code
    $(el).find('[data-action="reset"]').on('click', function (e) {
      e.preventDefault();

      var initialQuery = $(el).data('initial');
      $('.query-editable').text('');
      window.setTimeout(function() {
        $('.query-editable').text(initialQuery);
      }, 80);
    });

    $(el).find('.runnable-code').on('click', function () {
      $(this).find('.query-editable').focus();
    });
  });

  // Init code copy buttons for runnables
  var text;
  var clip = new Clipboard('[data-action="copy"]', {
    text: function(trigger) {
      text = $(trigger).closest('.runnable').text();
      return text.replace(/^\$\s/gm, "");
    }
  });

  clip.on("success", function(e) {
    e.clearSelection();
    $(e.trigger).text("Copied")
      .addClass('copied');

    window.setTimeout(function() {
      $(e.trigger).text("Copy").removeClass('copied');
    }, 2000);
  });

  // On page load
  updateSidebar();
  document.querySelector('.sub-topics .topic.active').scrollIntoView();
})();

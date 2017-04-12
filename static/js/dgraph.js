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

function formatJavaCode(code) {
  return code.replace(/"/g, '\\"')
             .replace(/\s+/g, ' ')
             .replace(/\n/g, ' ');
}

/********** Cookie helpers **/

function createCookie(name, val, days) {
  var expires = '';
  if (days) {
    var date = new Date();
    date.setTime(date.getTime() + (days * 24 * 60 * 60 * 1000));
    expires = '; expires=' + date.toUTCString();
  }

  document.cookie = name + '=' + val + expires + '; path=/';
}

function readCookie(name) {
  var nameEQ = name + '=';
  var ca = document.cookie.split(';');
  for (var i = 0; i < ca.length; i++) {
    var c = ca[i];
    while (c.charAt(0)==' ') c = c.substring(1,c.length);
    if (c.indexOf(nameEQ) == 0) return c.substring(nameEQ.length,c.length);
  }
  return null;
}

function eraseCookie(name) {
  createCookie(name, '', -1);
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
  // Initialize languages
  var preferredLang = readCookie('lang');
  if (preferredLang) {
    $('.runnable').each(function () {
      var $runnable = $(this);

      navToRunnableTab($runnable, preferredLang);
    });
  } else {
    createCookie('lang', 'curl', 365);
  }

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
    var topSideOffset = 120;

    var activeHash;
    for (var i = 0; i < allLinks.length; i++) {
      var h = allLinks[i];
      var hash = h.getElementsByTagName('a')[0].hash;

      if (h.offsetTop - topSideOffset > currentScrollY) {
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
    var id = heading.id;
    var anchorOffset = document.createElement('div');
    anchorOffset.className = 'anchor-offset';
    anchorOffset.id = id;

    var anchor = document.createElement("a");
    anchor.href = '#' + id;
    anchor.className = 'anchor';
    // anchor.innerHTML = 'link'
    anchor.innerHTML = '<i class="fa fa-link"></i>'
    heading.insertBefore(anchor, heading.firstChild);
    heading.insertBefore(anchorOffset, heading.firstChild);

    // Remove the id from heading
    // Instead we will assign the id to the .anchor-offset element to account
    // for the fixed header height
    heading.removeAttribute('id');
  }
  var h2s = document.querySelectorAll(
    '.content-wrapper h2, .content-wrapper h3');
  for (var i = 0; i < h2s.length; i++) {
    appendAnchor(h2s[i]);
  }

  // code collapse
  var pres = $('pre');
  pres.each(function() {
    var self = this;

    var isInRunnable = $(self).parents('.runnable').length > 0;
    if (isInRunnable) {
      return;
    }

    if (self.clientHeight > 330) {
      if (self.clientHeight < 380) {
        return;
      }

      self.className += " collapsed";

      var showMore = document.createElement("div");
      showMore.className = "showmore";
      showMore.innerHTML = "<span>Show all</span>";
      showMore.addEventListener("click", function() {
        self.className = "";
        showMore.parentNode.removeChild(showMore);
      });

      this.appendChild(showMore);
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

  // setupRunnableClipboard configures clipboard buttons for runnable
  // @params runnableEl {HTMLElement|JQueryElement} - HTML Element for runnable
  function setupRunnableClipboard(runnableEl) {
    // Set up clipboard
    var codeClipEl = $(runnableEl).find('.code-btn[data-action="copy-code"]')[0];
    var codeClip = new Clipboard(codeClipEl, {
      text: function(trigger) {
        var $runnable = $(trigger).closest('.runnable');
        var text = $runnable.find('.runnable-code .runnable-tab-content.active').text().trim();
        return text.replace(/^\$\s/gm, "");
      }
    });

    codeClip.on("success", function(e) {
      e.clearSelection();
      $(e.trigger).text("Copied")
        .addClass('copied');

      window.setTimeout(function() {
        $(e.trigger).text("Copy").removeClass('copied');
      }, 2000);
    });

    codeClip.on("error", function(e) {
      e.clearSelection();
      $(e.trigger).text("Error copying");

      window.setTimeout(function() {
        $(e.trigger).text("Copy");
      }, 2000);
    });

    var outputClipEl = $(runnableEl).find('.code-btn[data-action="copy-output"]')[0];
    var outputClip = new Clipboard(outputClipEl, {
      text: function(trigger) {
        var $runnable = $(trigger).closest('.runnable');
        var $output = $runnable.find('.output');

        var text = $output.text().trim() || ' ';
        return text;
      }
    });

    outputClip.on("success", function(e) {
      e.clearSelection();
      $(e.trigger).text("Copied")
        .addClass('copied');

      window.setTimeout(function() {
        $(e.trigger).text("Copy").removeClass('copied');
      }, 2000);
    });

    outputClip.on("error", function(e) {
      e.clearSelection();
      $(e.trigger).text("Error copying");

      window.setTimeout(function() {
        $(e.trigger).text("Copy");
      }, 2000);
    });
  }

  /**
   * launchRunnableModal launches a runnable in a modal and configures the
   * clipboard buttons
   *
   * @params runnabelEl {HTMLElement} - a runnable element
   * @params options {Object}
   * @params options.runnableClass {String} - the class name to apply to the
   *         '.runnable' div. Useful when launching the runnabe as editing mode.
   */
  function launchRunnableModal(runnabelEl, options) {
    // default argument
    options = typeof options !== 'undefined' ? options : {};

    var $originalRunnable = $(runnabelEl);
    var $modal = $('#runnable-modal');
    var $modalBody = $modal.find('.modal-body');

    // set inner html as runnable
    var str = $originalRunnable.prop('outerHTML');
    $modalBody.html(str);

    // show modal
    $modal.modal({
      keyboard: true
    });

    var $runnableEl = $modal.find('.runnable');
    if (options.runnableClass) {
      $runnableEl.addClass(options.runnableClass);
    }

    // intiailze clipboard
    setupRunnableClipboard($runnableEl);
  }

  /**
   * navToRunnableTab navigates to the target tab
   * @params targetTab {String}
   */
  function navToRunnableTab($runnable, targetTab) {
    // If needed, exit the edit mode
    if (targetTab !== 'edit' && $runnable.hasClass('editing')) {
      $runnable.removeClass('editing');
    }

    $runnable.find('.nav-languages .language.active').removeClass('active');
    $runnable.find('.language[data-target="' + targetTab + '"]').addClass('active');

    $runnable.find('.runnable-tab-content.active').removeClass('active');
    $runnable.find('.runnable-tab-content[data-tab="' + targetTab + '"]').addClass('active');
  }

  // changeLanguage changes the preferred programminng language for the examples
  // and navigate all example tabs to that language
  // @params language {String}
  function changeLanguage(language) {
    // First, set cookie
    createCookie('lang', language, 365);

    // Navigate all runnable tabs to the langauge
    $('.runnable').each(function () {
      var $runnable = $(this);

      navToRunnableTab($runnable, language);
    });
  }

  function initCodeMirror($runnable) {
    $runnable.find('.CodeMirror').remove();

    var editableEl = $runnable.find('.query-content-editable')[0];
    var cm = CodeMirror.fromTextArea(editableEl, {
      lineNumbers: true,
      autoCloseBrackets: true,
      lineWrapping: true,
      autofocus: true,
      tabSize: 2
    });

    cm.on('change', function (c) {
      var val = c.doc.getValue();
      $runnable.attr('data-unsaved', val);
      c.save();
    });
  }

  // updateQueryContents updates the query contents in all tabs
  function updateQueryContents($runnables, newQuery) {
    $runnables.find('.query-content').not('.java').text(newQuery);

    var javaTxt = formatJavaCode(newQuery);
    $runnables.find('.query-content.java').text(javaTxt);
  }

  function getLatencyTooltipHTML(serverLatencyInfo, networkLatency) {
    var contentHTML = '<div class="measurement-row"><div class="measurement-key">JSON:</div><div class="measurement-val">' + serverLatencyInfo.json + '</div></div><div class="measurement-row"><div class="measurement-key">Parsing:</div><div class="measurement-val">' + serverLatencyInfo.parsing + '</div></div><div class="measurement-row"><div class="measurement-key">Processing:</div><div class="measurement-val">' + serverLatencyInfo.processing + '</div></div><div class="divider"></div><div class="measurement-row"><div class="measurement-key total">Total:</div><div class="measurement-val">' + serverLatencyInfo.total + '</div></div>';
    var outputHTML = '<div class="latency-tooltip-container">' + contentHTML + '</div>';

    return outputHTML;
  }

  function getTotalServerLatencyInMS(serverLatencyInfo) {
    var totalServerLatency = serverLatencyInfo.total;

    var unit = totalServerLatency.slice(-2);
    var val = totalServerLatency.slice(0, -2);

    if (unit === 'µs') {
      return val / 1000;
    }

    // else assume 'ms'
    return val;
  }

  /**
   * updateLatencyInformation update the latency information displayed in the
   * $runnable.
   *
   * @params $runnable {JQueryElement}
   * @params serverLatencyInfo {Object} - latency info returned by the server
   * @params networkLatency {Number} - network latency in milliseconds
   */
  function updateLatencyInformation($runnable, serverLatencyInfo, networkLatency) {
    var isModal = $runnable.parents('#runnable-modal').length > 0;

    var totalServerLatency = getTotalServerLatencyInMS(serverLatencyInfo);
    var networkOnlyLatency = networkLatency - totalServerLatency;

    $runnable.find('.latency-info').removeClass('hidden');
    $runnable.find('.server-latency .number').text(serverLatencyInfo.total);
    $runnable.find('.network-latency .number').text(networkOnlyLatency + 'ms');

    var tooltipHTML = getLatencyTooltipHTML(serverLatencyInfo, networkOnlyLatency);

    $runnable.find('.server-latency-tooltip-trigger')
     .attr('title', tooltipHTML)
     .tooltip();
  }

  // Running code
  $(document).on('click', '.runnable [data-action="run"]', function (e) {
    e.preventDefault();

    // there can be at most two instances of a same runnable because users can
    // launch a runnable as a modal. they share the same checksum
    var checksum = $(this).closest('.runnable').data('checksum');
    var $currentRunnable = $(this).closest('.runnable');
    var $runnables = $('.runnable[data-checksum="' + checksum + '"]');
    var codeEl = $runnables.find('.output');
    var isModal = $currentRunnable.parents('#runnable-modal').length > 0;
    var query = $(this).closest('.runnable').attr('data-current');

    $runnables.find('.output-container').removeClass('empty error');
    codeEl.text('Waiting for the server response...');

    var startTime;
    $.post({
      url: 'https://play.dgraph.io/query?latency=true',
      data: query,
      dataType: 'json',
      beforeSend: function () {
        startTime = new Date().getTime();
      }
    })
    .done(function (res) {
      var now = new Date().getTime();
      var networkLatency = now - startTime;
      var serverLatencyInfo = res.server_latency;
      delete res.server_latency;

      // In some cases, the server does not return latency information
      // TODO: find better ways to check for errors or fix dgraph to make the
      // response consistent
      if (!res.code || !/Error/i.test(res.code)) {
        updateLatencyInformation($runnables, serverLatencyInfo, networkLatency);
      }

      var userOutput = JSON.stringify(res, null, 2);
      codeEl.text(userOutput);
      for (var i = 0; i < codeEl.length; i++) {
        hljs.highlightBlock(codeEl[i]);
      }

      if (!isModal) {
        var currentRunnableEl = $currentRunnable[0];
        launchRunnableModal(currentRunnableEl);
      }
    })
    .fail(function (xhr, status, error) {
      $runnables.find('.output-container').addClass('error');

      codeEl.text(xhr.responseText || error);
    });
  });

  // Refresh code
  $(document).on('click', '.runnable [data-action="reset"]', function (e) {
    e.preventDefault();

    var $runnable = $(this).closest('.runnable');
    var initialQuery = $runnable.data('initial');

    $runnable.attr('data-unsaved', initialQuery);
    $runnable.find('.query-content-editable').val(initialQuery).text(initialQuery);

    initCodeMirror($runnable);

    window.setTimeout(function() {
      $runnable.find('.query-content-editable').text(initialQuery);
    }, 80);
  });

  $(document).on('click', '.runnable [data-action="save"]', function (e) {
    e.preventDefault();

    var checksum = $(this).closest('.runnable').data('checksum');
    var $currentRunnable = $(this).closest('.runnable');
    var $runnables = $('.runnable[data-checksum="' + checksum + '"]');
    var newQuery = $currentRunnable.attr('data-unsaved') ||
                   $currentRunnable.attr('data-current');

    newQuery = newQuery.trim();

    // Update query examples and the textarea with the current query
    $runnables.attr('data-current', newQuery);
    updateQueryContents($runnables, newQuery);
    // We update the value as well as the inner text because when launched in
    // a modal, value will be lose as HTML is copied
    // TODO: implement JS object for runnable instead of storing these states
    // in DOM. Is there a good way to do so without framework?
    $runnables.find('.query-content-editable').val(newQuery).text(newQuery);

    var dest = readCookie('lang');
    navToRunnableTab($currentRunnable, dest);
  });

  $(document).on('click', '.runnable [data-action="discard"]', function (e) {
    e.preventDefault();

    var $runnable = $(this).closest('.runnable');

    // Restore to initial query
    var currentQuery = $runnable.attr('data-current');
    updateQueryContents($runnable, currentQuery);
    $runnable.find('.query-content-editable').val(currentQuery).text(currentQuery);

    var dest = readCookie('lang');
    navToRunnableTab($runnable, dest);
  });

  $(document).on('click', '.runnable [data-action="expand"]', function (e) {
    e.preventDefault();

    var $runnable = $(this).closest('.runnable');
    var runnableEl = $runnable[0];
    launchRunnableModal(runnableEl);
  });

  $(document).on('click', '.runnable [data-action="edit"]', function (e) {
    e.preventDefault();

    var $runnable = $(this).closest('.runnable');
    var isModal = $runnable.parents('#runnable-modal').length > 0;

    if (isModal) {
      $runnable.addClass('editing');
      navToRunnableTab($runnable, 'edit');
      initCodeMirror($runnable);
    } else {
      var currentRunnableEl = $runnable;
      launchRunnableModal(currentRunnableEl, { runnableClass: 'editing' });
    }
  });

  $(document).on('click', '.runnable [data-action="nav-lang"]', function (e) {
    e.preventDefault();
    var targetTab = $(this).data('target');
    var $runnable = $(this).closest('.runnable');

    changeLanguage(targetTab);
  });

  // Runnable modal event hooks
  $('#runnable-modal').on('hidden.bs.modal', function (e) {
    $(this).find('.server-latency-tooltip-trigger').tooltip('dispose');
    $(this).find('.modal-body').html('');
  });

  $('#runnable-modal').on('shown.bs.modal', function () {
    var $runnable = $(this).find('.runnable');

    // Focus the output so that it is scrollable by keyboard
    var $output = $(this).find('.output');
    $output.focus();

    // if .editing class is found on .runnable, we transition to the edit tab
    // and initialize the code mirror. Such transition and initialization should
    // be done when the modal has been completely transitioned, and therefore
    // we put this logic here instead of in launchRunnableModal function at the
    // cost of some added complexity
    var isEditing = $runnable.hasClass('editing');
    if (isEditing) {
      navToRunnableTab($runnable, 'edit')
      initCodeMirror($runnable);
    }

    var hasRun = !$runnable.find('.latency-info').hasClass('hidden');
    if (hasRun) {
      $runnable.find('.server-latency-tooltip-trigger').tooltip();
    }
  });

  /********** On page load **/
  updateSidebar();
  document.querySelector('.sub-topics .topic.active').scrollIntoView();

  // Initialize runnables
   $('.runnable').each(function () {
     // First, we reinitialize the query contents because some languages require
     // specific formatting
     var $runnable = $(this);
     var currentQuery = $runnable.attr('data-current');
     updateQueryContents($runnable, currentQuery);

     setupRunnableClipboard(this);
   });

  /********** Config **/

  // Get clipboard.js to work inside bootstrap modal
  // http://stackoverflow.com/questions/38398070/bootstrap-modal-does-not-work-with-clipboard-js-on-firefox
  $.fn.modal.Constructor.prototype._enforceFocus = function() {};
})();

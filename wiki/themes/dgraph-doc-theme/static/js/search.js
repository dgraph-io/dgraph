var lunrIndex, pagesIndex;

// Initialize lunrjs using our generated index file
function initLunr() {
    // First retrieve the index file
    $.getJSON(baseurl + "/json/search.json")
        .done(function(index) {
            pagesIndex = index;
            // Set up lunrjs by declaring the fields we use
            // Also provide their boost level for the ranking
            lunrIndex = new lunr.Index
            lunrIndex.ref("uri");
            lunrIndex.field('title', {
                boost: 15
            });
            lunrIndex.field('tags', {
                boost: 10
            });
            lunrIndex.field("content", {
                boost: 5
            });

            // Feed lunr with each file and let lunr actually index them
            pagesIndex.forEach(function(page) {
                lunrIndex.add(page);
            });
            lunrIndex.pipeline.remove(lunrIndex.stemmer)
        })
        .fail(function(jqxhr, textStatus, error) {
            var err = textStatus + ", " + error;
            console.error("Error getting Hugo index flie:", err);
        });
}

/**
 * Trigger a search in lunr and transform the result
 *
 * @param  {String} query
 * @return {Array}  results
 */
function search(query) {
    // Find the item in our index corresponding to the lunr one to have more info
    return lunrIndex.search(query).map(function(result) {
            return pagesIndex.filter(function(page) {
                return page.uri === result.ref;
            })[0];
        });
}

// Let's get started
initLunr();
$( document ).ready(function() {
    var horseyList = horsey($("#search-by").get(0), {
        suggestions: function (value, done) {
            var query = $("#search-by").val();
            var results = search(query);
            done(results);
        },
        filter: function (q, suggestion) {
            return true;
        },
        set: function (value) {
            location.href=value.href;
        },
        render: function (li, suggestion) {
            var uri = suggestion.uri.substring(1,suggestion.uri.length);
            var indexOfIndex = uri.lastIndexOf("/index");
            if (indexOfIndex == -1) {
                indexOfIndex = uri.length;
            }
            var href = uri.substring(uri.indexOf("/"), indexOfIndex);
            suggestion.href = baseurl + href;


            var query = $("#search-by").val();
            var numWords = 2;
            var text = suggestion.content.match("(?:\\s?(?:[\\w]+)\\s?){0,"+numWords+"}"+query+"(?:\\s?(?:[\\w]+)\\s?){0,"+numWords+"}");
            suggestion.context = text;
            var image = '<div>' + '» ' + suggestion.title + '</div><div style="font-size:12px">' + (suggestion.context || '') +'</div>';
            li.innerHTML = image;
        },
        limit: 10
    });
    horseyList.refreshPosition();
});

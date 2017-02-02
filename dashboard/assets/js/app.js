$(document).ready(function() {
  var nodes = [],
    edges = [],
    uidMap = {},
    network;

  function getLabel(properties) {
    var label = "";
    if (properties["name"] != undefined) {
      label = properties["name"]
    } else if (properties["name.en"] != undefined) {
      label = properties["name.en"]
    } else {
      label = properties["_uid_"]
    }

    words = label.split(" ")
    if (words.length > 1) {
      label = words[0] + "\n" + words[1]
      if (words.length > 2) {
        label += "..."
      }
    }
    return label
  }

  var predLabel = {};
  var edgeLabels = {};

  function getPredProperties(pred) {
    prop = predLabel[pred]
    if (prop != undefined) {
      // We have already calculated the label for this predicate.
      return prop
    }

    var l
    for (var i = 1; i <= pred.length; i++) {
      l = pred.substr(0, i)
      if (edgeLabels[l] == undefined) {
        // This label hasn't been allocated yet.
        predLabel[pred] = {
          'label': l,
          'color': randomColor()
        };
        edgeLabels[l] = true;
        break;
      }
      // If it has already been allocated, then we increase the substring length and look again.
    }
    if (l == undefined) {
      predLabel[pred] = {
        'label': pred,
        'color': randomColor()
      };
      edgeLabels[l] = true;
    }
    return predLabel[pred]
  }

  function processObject(obj, src) {
    var properties = {}
    for (var prop in obj) {
      // If its just a value, then we store it in properties for this node.
      if (!Array.isArray(obj[prop])) {
        properties[prop] = obj[prop]
      } else {
        var arr = obj[prop]
        for (var i = 0; i < arr.length; i++) {
          processObject(arr[i], {
            id: obj["_uid_"],
            pred: prop
          })
        }
      }

    }

    var predProperties = getPredProperties(src.pred)
    if (!uidMap[obj["_uid_"]]) {
      uidMap[obj["_uid_"]] = true

      nodes.push({
        id: obj["_uid_"],
        label: getLabel(properties),
        title: JSON.stringify(properties),
        group: src.pred
      })
    }
    if (src.pred != "") {
      edges.push({
        from: src.id,
        to: obj["_uid_"],
        title: src.pred,
        label: predProperties["label"],
        arrows: 'to'
      })
    }
  }

  $("form").on('submit', function(e) {
    e.preventDefault();
    $("#graph").addClass("hourglass");
    // TODO - When user is typing tell him he can press Ctrl + Enter to send query.
    // TODO - Clicking on node should show information regarding it.
    nodes = []
    edges = []
    uidMap = {}
    renderNetwork(nodes, edges)
    $("#server_latency").val("");
    $("#rendering").val("");

    $.ajax({
      url: "http://localhost:8080/query",
      method: "POST",
      data: $("#query").val(),
      dataType: 'json',
      success: function(result) {
        $("#graph").removeClass("hourglass");
        $("#graph").removeClass("error-res");
        $("#graph").removeClass("success-res");
        response = result.debug
        if (response != undefined) {
          $("#server_latency").val(result.server_latency.total);
          var startTime = new Date();
          for (var i = 0; i < response.length; i++) {
            processObject(response[i], {
              id: "",
              pred: "debug",
            })
          }
          renderNetwork(nodes, edges)
          var endTime = new Date();
          var timeTaken = (endTime.getTime() - startTime.getTime()) / 1000;
          $("#rendering").val(timeTaken + "s");
        } else if (result.code != undefined) {
          $("#graph").addClass("success-res");
          $("#graph").text(JSON.stringify(result, null, 2));
        } else {
          $("#graph").addClass("error-res");
          $("#graph").text("Only queries with debug as root work for now.")
        }
      },
      error: function(jqXHR, textStatus, errorThrown) {
        $("#graph").removeClass("success-res");
        $("#graph").removeClass("hourglass");
        $("#graph").addClass("error-res");
        $("#graph").text(jqXHR.responseText);
      },
      // Timeout of 60 seconds.
      timeout: 6000,
    });
  })


  // create a network
  function renderNetwork(nodes, edges) {
    var container = document.getElementById('graph');
    var data = {
      nodes: new vis.DataSet(nodes),
      edges: new vis.DataSet(edges)
    };
    var options = {
      nodes: {
        shape: 'circle',
        font: {
          size: 16
        },
        margin: {
          top: 25
        }
      },
      height: '100%',
      width: '100%',
      interaction: {
        hover: true,
        tooltipDelay: 1000
      },
    };
    network = new vis.Network(container, data, options);

    // network.on("click", function(params) {
    //   params.event = "[original event]";
    //   console.log(JSON.stringify(params, null, 2));
    // });
  }

  $('#query').keydown(function(e) {
    // Ctrl + Enter submits the query.
    if (e.ctrlKey && e.keyCode == 13) {
      if ($("#query").val() != "") {
        $("#query-form").submit();
      }
    }
  });
})

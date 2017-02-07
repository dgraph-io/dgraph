var React = require('react');
var ReactRouter = require('react-router');
var Link = ReactRouter.Link;
require('whatwg-fetch');
var vis = require('vis');
var Glyphicon = require('react-bootstrap').Glyphicon;

require('brace');
var AceEditor = require('react-ace').default;
require('brace/mode/logiql');
require('brace/theme/github');

var NavBar = require('./Navbar');

var styles = require('../styles');
var graph = styles.graph;
var stats = styles.statistics;
var editor = styles.editor;
var properties = styles.properties;
var fullscreen = styles.fullscreen;

// TODO - Abstract these out and remove these from global scope.
var network,
  // We will store the vis data set in these nodes.
  nodeSet,
  edgeSet;

// TODO - Move these three functions to server or to some other component. They dont
// really belong here.
function getLabel(properties) {
  var label = "";
  if (properties["name"] != undefined) {
    label = properties["name"]
  } else if (properties["name.en"] != undefined) {
    label = properties["name.en"]
  } else {
    label = properties["_uid_"]
  }

  var words = label.split(" "),
    firstWord = words[0];
  if (firstWord.length > 20) {
    label = [firstWord.substr(0, 9), firstWord.substr(9, 17) + "..."].join("-\n")
  } else if (firstWord.length > 10) {
    label = [firstWord.substr(0, 9), firstWord.substr(9, firstWord.length)].join("-\n")
  } else {
    // First word is less than 10 chars so we can display it in full.
    if (words.length > 1) {
      if (words[1].length > 10) {
        label = [firstWord, words[1] + "..."].join("\n")
      } else {
        label = [firstWord, words[1]].join("\n")
      }
    } else {
      label = firstWord
    }
  }


  return label
}

// This function shortens and calculates the label for a predicate.
function getPredProperties(pred, predLabel, edgeLabels) {
  var prop = predLabel[pred]
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
        // 'color': randomColor()
      };
      edgeLabels[l] = true;
      break;
    }
    // If it has already been allocated, then we increase the substring length and look again.
  }
  if (l == undefined) {
    predLabel[pred] = {
      'label': pred,
      // 'color': randomColor()
    };
    edgeLabels[l] = true;
  }
  return predLabel[pred]
}

function processGraph(response, root, maxNodes) {
  var nodesStack = [],
    predLabel = {},
    edgeLabels = {},
    uidMap = {},
    nodes = [],
    edges = [],
    emptyNode = {
      node: {},
      src: {
        id: "",
        pred: "empty"
      }
    };

  for (var i = 0; i < response.length; i++) {
    nodesStack.push({
      node: response[i],
      src: {
        id: "",
        pred: root
      }
    })
  };

  // We push an empty node after all the children. This would help us know when
  // we have traversed all nodes at a level.
  nodesStack.push(emptyNode)

  while (nodesStack.length > 0) {
    var obj = nodesStack.shift()
      // Check if this is an empty node.
    if (Object.keys(obj.node).length === 0 && obj.src.pred === "empty") {
      // We break out if we have reached the max node limit.
      if (nodesStack.length == 0 || (maxNodes != -1 && nodes.length > maxNodes)) {
        break
      } else {
        nodesStack.push(emptyNode)
        continue
      }
    }

    var properties = {}

    for (var prop in obj.node) {
      // If its just a value, then we store it in properties for this node.
      if (!Array.isArray(obj.node[prop])) {
        properties[prop] = obj.node[prop]
      } else {
        var arr = obj.node[prop]
        for (var i = 0; i < arr.length; i++) {
          nodesStack.push({
            node: arr[i],
            src: {
              pred: prop,
              id: obj.node["_uid_"]
            }
          })
        }
      }
    }

    if (!uidMap[properties["_uid_"]]) {
      uidMap[properties["_uid_"]] = true
      nodes.push({
        id: properties["_uid_"],
        label: getLabel(properties),
        title: JSON.stringify(properties),
        group: obj.src.pred,
        value: 1
      })
    }

    var predProperties = getPredProperties(obj.src.pred, predLabel, edgeLabels)
    if (obj.src.id != "") {
      edges.push({
        id: [obj.src.id, properties["_uid_"]].join("-"),
        from: obj.src.id,
        to: properties["_uid_"],
        title: obj.src.pred,
        label: predProperties["label"],
        arrows: 'to'
      })
    }
  }

  return [nodes, edges];
}

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
      scaling: {
        max: 20,
        min: 20,
        label: {
          enabled: true,
          min: 14,
          max: 14
        }
      },
      font: {
        size: 16
      },
      margin: {
        top: 25
      },
    },
    height: '100%',
    width: '100%',
    interaction: {
      hover: true,
      tooltipDelay: 1000
    },
    layout: {
      improvedLayout: nodes.length < 50
    }
  };

  network = new vis.Network(container, data, options);
  var that = this;
  network.on("click", function(params) {
    if (params.nodes.length > 0) {
      var nodeUid = params.nodes[0],
        currentNode = nodeSet.get(nodeUid);

      that.setState({
        currentNode: currentNode.title
      });

      var outgoing = data.edges.get({
        filter: function(node) {
          return node.from === nodeUid
        }
      })
      var expanded = outgoing.length > 0

      var outgoingEdges = edgeSet.get({
        filter: function(node) {
          return node.from === nodeUid
        }
      })

      var adjacentNodeIds = outgoingEdges.map(function(edge) {
        return edge.to
      })


      var adjacentNodes = nodeSet.get(adjacentNodeIds)
        // TODO -See if we can set a meta property to a node to know that its
        // expanded or closed and avoid this computation.
      if (expanded) {
        data.nodes.remove(adjacentNodeIds)
        data.edges.remove(outgoingEdges.map(function(edge) {
          return edge.id
        }))
      } else {
        var updatedNodeIds = data.nodes.update(adjacentNodes)
        var updatedEdgeids = data.edges.update(outgoingEdges)
      }
    }
  });

  network.on("dragEnd", function(params) {
    for (var i = 0; i < params.nodes.length; i++) {
      var nodeId = params.nodes[i];
      data.nodes.update({ id: nodeId, fixed: { x: true, y: true } });
    }
  });

  network.on('dragStart', function(params) {
    for (var i = 0; i < params.nodes.length; i++) {
      var nodeId = params.nodes[i];
      data.nodes.update({ id: nodeId, fixed: { x: false, y: false } });
    }
  });
}

function checkStatus(response) {
  if (response.status >= 200 && response.status < 300) {
    return response
  } else {
    var error = new Error(response.statusText)
    error.response = response
    throw error
  }
}

function parseJSON(response) {
  return response.json()
}

function timeout(ms, promise) {
  return new Promise(function(resolve, reject) {
    setTimeout(function() {
      reject(new Error("timeout"))
    }, ms)
    promise.then(resolve, reject)
  })
}

var Home = React.createClass({
  getInitialState: function() {
    return {
      query: '',
      lastQuery: '',
      response: '',
      latency: '',
      rendering: '',
      resType: '',
      currentNode: '',
      nodes: 0,
      relations: 0
    }
  },
  queryChange: function(newValue) {
    // TODO - Maybe store this at a better place once you figure it out.
    this.query = newValue;
  },
  // TODO - Dont send mutation. query if text didn't change.
  runQuery: function(e) {
    e.preventDefault();

    if (this.query === this.state.lastQuery) {
      return;
    }
    // Resetting state
    predLabel = {}, edgeLabels = {}, uidMap = {}, nodes = [], edges = [];
    network && network.destroy();
    this.setState(Object.assign(this.getInitialState(), {
      resType: 'hourglass',
      lastQuery: this.query
    }));

    var that = this;
    timeout(60000, fetch('http://localhost:8080/query?debug=true', {
        method: 'POST',
        mode: 'cors',
        headers: {
          'Content-Type': 'text/plain',
        },
        body: this.query
      }).then(checkStatus)
      .then(parseJSON)
      .then(function(result) {
        that.setState({
          response: '',
          resType: ''
        })
        var key = Object.keys(result)[0];
        if (result.code != undefined && result.message != undefined) {
          // This is the case in which user sends a mutation. We display the response from server.
          that.setState({
            resType: 'success-res',
            response: JSON.stringify(result, null, 2)
          })
        } else if (key != undefined) {
          // We got the result for a query.
          var response = result[key]
          that.setState({
            'latency': result.server_latency.total
          });
          var startTime = new Date();

          setTimeout(function() {
            var graph = processGraph(response, key, -1);
            nodeSet = new vis.DataSet(graph[0])
            edgeSet = new vis.DataSet(graph[1])
            that.setState({
              nodes: graph[0].length,
              relations: graph[1].length
            });
          }.bind(that), 1000)

          // We call procesGraph with a 40 node limit and calculate the whole dataset in
          // the background.
          var graph = processGraph(response, key, 40);
          setTimeout(function() {
            that.setState({
              partial: this.state.nodes > graph[0].length
            })
          }.bind(that), 1000)

          renderNetwork.call(that, graph[0], graph[1]);

          var endTime = new Date();
          var timeTaken = (endTime.getTime() - startTime.getTime()) / 1000;
          that.setState({
            'rendering': timeTaken + 's'
          });
        } else {
          console.warn("We shouldn't be here really")
            // We probably didn't get any results.
          that.setState({
            resType: 'error-res',
            response: "Your query did not return any results."
          })
        }
      })).catch(function(error) {
      console.log(error.stack)
      var err = error.response && error.response.text() || error.message
      return err
    }).then(function(errorMsg) {
      if (errorMsg != undefined) {
        that.setState({
          response: errorMsg,
          resType: 'error-res'
        })
      }
    })
  },
  render: function() {
    return (
      <div>
      <NavBar></NavBar>
    <div className="container">
        <div className="row justify-content-md-center">
            <div className="col-md-offset-1 col-md-10">
                <form id="query-form">
                    <div className="form-group">
                        <label htmlFor="query">Query</label>
                        <button type="submit" className="btn btn-primary pull-right" onClick={this.runQuery}>Run</button>
                    </div>
                </form>
      <div className="editor" style={editor}>
      <AceEditor
        mode="logiql"
        theme="github"
        name="editor"
        editorProps={{$blockScrolling: true}}
        width='100%'
        height='300px'
        fontSize='16px'
        value={this.query}
        focus={true}
        showPrintMargin={false}
        wrapEnabled={true}
        onChange={this.queryChange}
        commands={[{
          name: 'runQuery',
          bindKey: {
            win: 'Ctrl-Enter',
            mac: 'Command-Enter'
          },
          exec: function(editor) {
            this.runQuery(new Event(''));
          }.bind(this)
        }]}
      />
      </div>
          <label> Response </label>
        <div>Nodes: {this.state.nodes}, Edges: {this.state.relations}</div>
        <div style={{height:'20px'}}>{this.state.partial == true ? 'We have only loaded a subset of the graph. Click on a leaf node to expand its child nodes.': ''}</div>
        <div style={properties} title={this.state.currentNode}><span>Selected Node: <em>{this.state.currentNode}</em></span></div>
        <div style={graph} id="graph" className={this.state.resType}>{this.state.response}
          {/* <span style={fullscreen} onClick={activateFullScreen} className="glyphicon glyphicon-glyphicon glyphicon-resize-full">
           </span>
          */}
        </div>
          <div style={stats}>
              <form>
                      <div className="form-group col-sm-6">
                          <label htmlFor="server_latency">Server Latency</label>
                          <input type="input" className="form-control" value={this.state.latency} readOnly/>
                      </div>
                      <div className="form-group col-sm-6">
                          <label htmlFor="rendering">Rendering</label>
                          <input type="input" className="form-control" value={this.state.rendering} readOnly/>
                      </div>
              </form>
          </div>
        </div>
      </div>
    </div>
      </div>
    );
  }
});

module.exports = Home;

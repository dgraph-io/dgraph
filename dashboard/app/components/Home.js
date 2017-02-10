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
var screenfull = require('screenfull');
var classNames = require('classnames');
var _ = require('lodash');

var NavBar = require('./Navbar');
var Stats = require('./Stats');
var Query = require('./Query');

var styles = require('../styles');
var graph = styles.graph;
var stats = styles.statistics;
var editor = styles.editor;
var properties = styles.properties;
var fullscreen = styles.fullscreen;
var tip = styles.tip;

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

function hasChildren(node) {
  for (var prop in node) {
    if (Array.isArray(node[prop])) {
      return true
    }
  }
  return false
}

function hasProperties(props) {
  // Each node will have a _uid_. We check if it has other properties.
  return Object.keys(props).length !== 1
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

  var someNodeHasChildren = false;
  var ignoredChildren = [];

  for (var i = 0; i < response.length; i++) {
    var n = {
      node: response[i],
      src: {
        id: "",
        pred: root
      }
    };
    if (hasChildren(response[i])) {
      someNodeHasChildren = true;
      nodesStack.push(n);
    } else {
      ignoredChildren.push(n);
    }
  };

  if (!someNodeHasChildren) {
    nodesStack.push.apply(nodesStack, ignoredChildren)
  }

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

    var properties = {},
      hasChildNodes = false;

    for (var prop in obj.node) {
      // If its just a value, then we store it in properties for this node.
      if (!Array.isArray(obj.node[prop])) {
        properties[prop] = obj.node[prop]
      } else {
        hasChildNodes = true;
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
      if (hasProperties(properties) || hasChildNodes) {
        nodes.push({
          id: properties["_uid_"],
          label: getLabel(properties),
          title: JSON.stringify(properties, null, 2),
          group: obj.src.pred,
          value: 1
        })
      }
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

var doubleClickTime = 0;
var threshold = 200;

function doOnClick(params) {
  console.log("in do on click")
  if (params.nodes.length > 0) {
    var nodeUid = params.nodes[0],
      currentNode = nodeSet.get(nodeUid);

    this.setState({
      currentNode: currentNode.title,
      selectedNode: true
    });
  } else {
    this.setState({
      selectedNode: false,
      currentNode: '{}'
    })
  }
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
      keyboard: {
        enabled: true,
        bindToWindow: false
      },
      tooltipDelay: 1000000
    },
    layout: {
      improvedLayout: true
    },
    physics: {
      timestep: 0.8,
      barnesHut: {
        // avoidOverlap: 0.8,
        // springConstant: 0.1,
        damping: 0.3
      }
    }
  };

  var nodes = [],
    edges = [];

  network = new vis.Network(container, data, options);
  var that = this;

  network.on("doubleClick", function(params) {
    doubleClickTime = new Date();
    if (params.nodes && params.nodes.length > 0) {
      var nodeUid = params.nodes[0],
        currentNode = nodeSet.get(nodeUid);

      network.unselectAll();
      that.setState({
        currentNode: currentNode.title,
        selectedNode: false
      });

      var incoming = data.edges.get({
        filter: function(node) {
          return node.to === nodeUid
        }
      })

      if (incoming.length === 0) {
        // Its a root node, we don't want to expand/collapse.
        return
      }

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



  network.on("click", function(params) {
    var t0 = new Date();
    if (t0 - doubleClickTime > threshold) {
      setTimeout(function() {
        if (t0 - doubleClickTime > threshold) {
          doOnClick.bind(that)(params);
        }
      }, threshold);
    }
  });


  window.onresize = function() { network.fit(); }
  network.on("hoverNode", function(params) {
    // Only change properties if no node is selected.
    if (that.state.selectedNode) {
      return;
    }
    if (params.node === undefined) {
      return;
    }
    var nodeUid = params.node,
      currentNode = nodeSet.get(nodeUid);

    that.setState({
      currentNode: currentNode.title
    });
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
    var response = this.lastQuery()
    return {
      selectedNode: false,
      queryIndex: response[0],
      query: response[1],
      // We store the queries run in state, so that they can be displayed
      // to the user.
      queries: response[2],
      lastQuery: '',
      response: '',
      latency: '',
      rendering: '',
      resType: '',
      currentNode: '{}',
      nodes: 0,
      relations: 0,
      graph: '',
      graphHeight: 'fixed-height',
    }
  },
  updateQuery: function(e) {
    e.preventDefault();
    this.setState({
      query: e.target.innerHTML
    });
    window.scrollTo(0, 0);
    this.refs.code.editor.focus();
    this.refs.code.editor.navigateFileEnd();
    network && network.destroy();
  },
  // Handler which listens to changes on AceEditor.
  queryChange: function(newValue) {
    this.setState({ query: newValue });
  },
  resetState: function() {
    return {
      response: '',
      selectedNode: false,
      latency: '',
      rendering: '',
      resType: '',
      currentNode: '{}',
      nodes: 0,
      relations: 0,
      resType: 'hourglass',
      lastQuery: this.state.query
    }
  },
  storeQuery: function(query) {
    var queries = JSON.parse(localStorage.getItem("queries")) || [];
    queries.unshift(query);

    // (queries.length > 20) && (queries = queries.splice(0, 20))

    this.setState({
      queryIndex: queries.length - 1,
      queries: queries
    })
    localStorage.setItem("queries", JSON.stringify(queries));
  },
  lastQuery: function() {
    var queries = JSON.parse(localStorage.getItem("queries"))
    if (queries == null) {
      return [undefined, "", []]
    }
    // This means queries has atleast one element.

    return [0, queries[0], queries]
  },
  nextQuery: function() {
    var queries = JSON.parse(localStorage.getItem("queries"))
    if (queries == null) {
      return
    }

    var idx = this.state.queryIndex;
    if (idx === undefined || idx - 1 < 0) {
      return
    }
    this.setState({
      query: queries[idx - 1],
      queryIndex: idx - 1
    });
  },
  previousQuery: function() {
    var queries = JSON.parse(localStorage.getItem("queries"))
    if (queries == null) {
      return
    }

    var idx = this.state.queryIndex;
    if (idx === undefined || (idx + 1) === queries.length) {
      return
    }
    this.setState({
      queryIndex: idx + 1,
      query: queries[idx + 1]
    })
  },
  runQuery: function(e) {
    e.preventDefault();

    // if (this.state.query === this.state.lastQuery && this.state.resType == "") {
    //   return;
    // }

    // Resetting state
    network && network.destroy();
    this.setState(this.resetState());

    var that = this;
    timeout(60000, fetch('http://localhost:8080/query?debug=true', {
        method: 'POST',
        mode: 'cors',
        headers: {
          'Content-Type': 'text/plain',
        },
        body: this.state.query
      }).then(checkStatus)
      .then(parseJSON)
      .then(function(result) {
        that.setState({
          response: '',
          resType: ''
        })
        var key = Object.keys(result)[0];
        if (result.code != undefined && result.message != undefined) {
          that.storeQuery(that.state.query);
          // This is the case in which user sends a mutation. We display the response from server.
          that.setState({
            resType: 'success-res',
            response: JSON.stringify(result, null, 2)
          })
        } else if (key != undefined) {
          that.storeQuery(that.state.query);
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
          var graph = processGraph(response, key, 20);
          setTimeout(function() {
            that.setState({
              partial: this.state.nodes > graph[0].length
            })
          }.bind(that), 1000)

          renderNetwork.call(that, graph[0], graph[1]);

          var endTime = new Date();
          var timeTaken = ((endTime.getTime() - startTime.getTime()) / 1000).toFixed(2);
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
  enterFullScreen: function(e) {
    e.preventDefault();
    document.addEventListener(screenfull.raw.fullscreenchange, () => {
      if (!screenfull.isFullscreen) {
        this.setState({
          graph: '',
          graphHeight: 'fixed-height'
        })
      } else {
        // In full screen mode, we display the properties as a tooltip.
        this.setState({
          graph: 'fullscreen',
          graphHeight: 'full-height'
        });
      }
    });
    screenfull.request(document.getElementById('response'));
    // screenfull.request(document.getElementById('properties'));
  },
  render: function() {
    var graphClass = classNames({ 'fullscreen': this.state.graph === 'fullscreen' }, { 'graph': this.state.graph !== 'fullscreen' }, { 'error-res': this.state.resType == 'error-res' }, { 'success-res': this.state.resType == 'success-res' })
    return (
      <div>
      <NavBar></NavBar>
    <div className="container-fluid">
        <div className="row justify-content-md-center">
            <div className="col-sm-12">
              <div className="col-sm-5">
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
                    editorProps={{$blockScrolling: Infinity}}
                    width='100%'
                    height='350px'
                    fontSize='12px'
                    value={this.state.query}
                    focus={true}
                    ref="code"
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
                    },
                    {
                      name: 'previousQuery',
                      bindKey: {
                        win: 'Ctrl-Up',
                        mac: 'Command-Up'
                      },
                      exec: function(editor) {
                        this.previousQuery();
                      }.bind(this)
                    },
                    {
                      name: 'nextQuery',
                      bindKey: {
                        win: 'Ctrl-Down',
                        mac: 'Command-Down'
                      },
                      exec: function(editor) {
                        this.nextQuery();
                      }.bind(this)
                    }]}
                  />
                </div>
                <div style={tip}>Tips:<br/>
                  Ctrl + Enter to execute the query.<br/>
                  Ctrl + Up/Down arrow key to see previously run queries.
                </div>
                <Stats rendering={this.state.rendering} latency={this.state.latency} class="hidden-xs"></Stats>
              </div>
            <div className="col-sm-7">
              <label> Response </label>
                            {
                screenfull.enabled  &&
              <div style={fullscreen} className="pull-right" onClick={this.enterFullScreen}>
                <span style={{fontSize: '20px',padding:'5px'}} className="glyphicon glyphicon-glyphicon glyphicon-resize-full">
                </span>
              </div>
              }
              <div style={{width: '100%', height: '100%', padding: '5px'}} id="response">
              <div style={graph} className={this.state.graphHeight}>
              <div id="graph" className={graphClass}>{this.state.response}</div>
              </div>
              <div>Nodes: {this.state.nodes}, Edges: {this.state.relations}</div>
              <div style={{height:'auto'}}>{this.state.partial == true ? 'We have only loaded a subset of the graph. Click on a leaf node to expand its child nodes.': ''}</div>
              <div id="properties" style={{marginTop: '10px'}}>Current Node:<div style={properties} title={this.state.currentNode}><em><pre>{JSON.stringify(JSON.parse(this.state.currentNode), null, 2)}</pre></em></div>
              </div>
              </div>

            </div>
            <Stats rendering={this.state.rendering} latency={this.state.latency} class="visible-xs"></Stats>
            </div>
            </div>
            <div className="row">
              <div className="col-sm-12">
            <div style={{marginTop: '10px', borderTop: '1px solid black'}}>
            <h3 style={{marginLeft: '50px'}}>Previous Queries</h3>
            <div style={{maxHeight: '500px',overflowY: 'scroll', border: '1px solid black', margin: '0 50px'}}>
            {this.state.queries.map(function (query, i) {
              return <Query text={query} update={this.updateQuery} key={i}></Query>;
            },this)}
            </div>
            </div>
            </div>
            </div>
          </div> 
        </div>
    );
  }
});

module.exports = Home;

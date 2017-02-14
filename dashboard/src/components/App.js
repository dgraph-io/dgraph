// @flow

import React from 'react';

import vis from 'vis';
import 'brace';
import AceEditor from 'react-ace';
import 'brace/mode/logiql';
import 'brace/theme/github';
import screenfull from 'screenfull';
import classNames from 'classnames';

import NavBar from './Navbar';
import Stats from './Stats';
import Query from './Query';

import '../assets/css/App.css'


type Edge = {| id: string, from: string, to: string, arrows: string, label: string, title: string |}
type Node = {| id: string, label: string, title: string, group: string, value: number |}

// Visjs network
var network,
  globalNodeSet,
  globalEdgeSet;

// TODO - Move these three functions to server or to some other component. They dont
// really belong here.
function getLabel(properties: Object): string {
  var label = "";
  if (properties["name"] !== undefined) {
    label = properties["name"]
  } else if (properties["name.en"] !== undefined) {
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

type MapOfStrings = { [key: string]: string };
type MapOfBooleans = { [key: string]: boolean};

// This function shortens and calculates the label for a predicate.
function getPredProperties(pred: string, predLabel: MapOfStrings,
    edgeLabels: MapOfBooleans): string {
  var prop = predLabel[pred]
  if (prop !== undefined) {
    // We have already calculated the label for this predicate.
    return prop
  }

  var l
  for (var i = 1; i <= pred.length; i++) {
    l = pred.substr(0, i)
    if (edgeLabels[l] === undefined) {
      // This label hasn't been allocated yet.
      predLabel[pred] = l
      edgeLabels[l] = true;
      break;
    }
    // If it has already been allocated, then we increase the substring length and look again.
  }
  if (l === undefined) {
    predLabel[pred] = pred;
    edgeLabels[pred] = true;
  }
  return predLabel[pred]
}

function hasChildren(node: Object): boolean{
  for (var prop in node) {
    if (Array.isArray(node[prop])) {
      return true
    }
  }
  return false
}

function hasProperties(props: Object): boolean {
  // Each node will have a _uid_. We check if it has other properties.
  return Object.keys(props).length !== 1
}

type Src = {| id: string, pred: string |};
type ResponseNode = {| node: Object, src: Src |};

function processGraph(response: Object, root: string, maxNodes: number) {
  let nodesStack: Array <ResponseNode> = [],
    // Contains map of a lable to its shortform thats displayed.
    predLabel: MapOfStrings = {},

    // Stores the map of a label to boolean (only true values are stored).
    // This helps quickly find if a label has already been assigned.
    edgeLabels : MapOfBooleans = {},
    // Map of whether a Node with an Uid has already been created. This helps
    // us avoid creating duplicating nodes while parsing the JSON structure
    // which is a tree.
    uidMap : MapOfBooleans = {},
    nodes: Array <Node> = [],
    edges: Array <Edge> = [],
    emptyNode: ResponseNode = {
      node: {},
      src: {
        id: "",
        pred: "empty"
      }
    },
    someNodeHasChildren: boolean = false,
    ignoredChildren: Array <ResponseNode> = [];

  for (let i = 0; i < response.length; i++) {
    let n: ResponseNode = {
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
    let obj = nodesStack.shift()
      // Check if this is an empty node.
    if (Object.keys(obj.node).length === 0 && obj.src.pred === "empty") {
      // We break out if we have reached the max node limit.
      if (nodesStack.length === 0 || (maxNodes !== -1 && nodes.length > maxNodes)) {
        break
      } else {
        nodesStack.push(emptyNode)
        continue
      }
    }

    let properties: MapOfStrings = {},
      hasChildNodes: boolean = false;

    for (let prop in obj.node) {
      // If its just a value, then we store it in properties for this node.
      if (!Array.isArray(obj.node[prop])) {
        properties[prop] = obj.node[prop]
      } else {
        hasChildNodes = true;
        let arr = obj.node[prop]
        for (let i = 0; i < arr.length; i++) {
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
        var n: Node = {
          id: properties["_uid_"],
          label: getLabel(properties),
          title: JSON.stringify(properties, null, 2),
          group: obj.src.pred,
          value: 1
        }
		nodes.push(n)
      }
    }

    let predProperties = getPredProperties(obj.src.pred, predLabel, edgeLabels)
    if (obj.src.id !== "") {
      var e: Edge = {
        id: [obj.src.id, properties["_uid_"]].join("-"),
        from: obj.src.id,
        to: properties["_uid_"],
        title: obj.src.pred,
        label: predProperties,
        arrows: 'to'
      }
      edges.push(e)
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
      currentNode = globalNodeSet.get(nodeUid);

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
function renderNetwork(nodes: Array <Node>, edges: Array <Edge>) {
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

  network = new vis.Network(container, data, options);
  let that = this;

  network.on("doubleClick", function(params) {
    doubleClickTime = new Date();
    if (params.nodes && params.nodes.length > 0) {
      let nodeUid = params.nodes[0],
        currentNode = globalNodeSet.get(nodeUid);

      network.unselectAll();
      that.setState({
        currentNode: currentNode.title,
        selectedNode: false
      });

      let outgoing = data.edges.get({
        filter: function(node) {
          return node.from === nodeUid
        }
      })

      let expanded: boolean = outgoing.length > 0

      let outgoingEdges = globalEdgeSet.get({
        filter: function(node) {
          return node.from === nodeUid
        }
      })

      let adjacentNodeIds: Array <string> = outgoingEdges.map(function(edge) {
        return edge.to
      })


      let adjacentNodes = globalNodeSet.get(adjacentNodeIds)
        // TODO -See if we can set a meta property to a node to know that its
        // expanded or closed and avoid this computation.
      if (expanded) {
        data.nodes.remove(adjacentNodeIds)
        data.edges.remove(outgoingEdges.map(function(edge) {
          return edge.id
        }))
      } else {
        data.nodes.update(adjacentNodes)
        data.edges.update(outgoingEdges)
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
    let nodeUid: string = params.node,
      currentNode = globalNodeSet.get(nodeUid);

    that.setState({
      currentNode: currentNode.title
    });
  });

  network.on("dragEnd", function(params) {
    for (let i = 0; i < params.nodes.length; i++) {
      let nodeId: string = params.nodes[i];
      data.nodes.update({ id: nodeId, fixed: { x: true, y: true } });
    }
  });

  network.on('dragStart', function(params) {
    for (let i = 0; i < params.nodes.length; i++) {
      let nodeId: string = params.nodes[i];
      data.nodes.update({ id: nodeId, fixed: { x: false, y: false } });
    }
  });
}

function checkStatus(response) {
  if (response.status >= 200 && response.status < 300) {
    return response
  } else {
    let error = new Error(response.statusText)
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

type State = {
  selectedNode: boolean,
  partial: boolean,
  queryIndex: number,
  query: string,
  queries: Array <string>,
  lastQuery: string,
  response: string,
  latency: string,
  rendering: string,
  resType: string,
  currentNode: string,
  nodes: number,
  relations: number,
  graph: string,
  graphHeight: string
};

type Props = {
}

class App extends React.Component {
  state: State;

  constructor(props: Props) {
    super(props);
    let response = this.lastQuery()

    this.state = {
      selectedNode: false,
      partial: false,
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
    };
  }

  updateQuery = (e: Event) => {
    e.preventDefault();
    if(e.target instanceof HTMLElement){
        this.setState({
          query: e.target.dataset.query,
          rendering: '',
          latency: '',
          nodes: 0,
          relations: 0,
          selectedNode: false,
          partial: false,
          currentNode: '{}'
        });
    }
    window.scrollTo(0, 0);
    this.refs.code.editor.focus();
    this.refs.code.editor.navigateFileEnd();

    network && network.destroy();
  }

  // Handler which listens to changes on AceEditor.
  queryChange = (newValue: string) => {
    this.setState({ query: newValue });
  }

  resetState = () => {
    return {
      response: '',
      selectedNode: false,
      latency: '',
      rendering: '',
      currentNode: '{}',
      nodes: 0,
      relations: 0,
      resType: 'hourglass',
      lastQuery: this.state.query
    }
  }

  storeQuery = (query: string) => {
    let queries: Array <string> = JSON.parse(localStorage.getItem("queries") || '[]');
    queries.unshift(query);

    // (queries.length > 20) && (queries = queries.splice(0, 20))

    this.setState({
      queryIndex: queries.length - 1,
      queries: queries
    })
    localStorage.setItem("queries", JSON.stringify(queries));
  }

  lastQuery = () => {
    let queries: Array <string> = JSON.parse(localStorage.getItem("queries") || '[]')
    if (queries.length === 0) {
      return [-1, "", []]
    }
    // This means queries has atleast one element.

    return [0, queries[0], queries]
  }

  nextQuery = () => {
    let queries: Array <string> = JSON.parse(localStorage.getItem("queries") || '[]')
    if (queries.length === 0) {
      return
    }

    let idx: number = this.state.queryIndex;
    if (idx === -1 || idx - 1 < 0) {
      return
    }
    this.setState({
      query: queries[idx - 1],
      queryIndex: idx - 1
    });
  }

  previousQuery = () => {
    var queries: Array <string> = JSON.parse(localStorage.getItem("queries") || '[]')
    if (queries === '[]') {
      return
    }

    var idx = this.state.queryIndex;
    if (idx === -1 || (idx + 1) === queries.length) {
      return
    }
    this.setState({
      queryIndex: idx + 1,
      query: queries[idx + 1]
    })
  }

  runQuery = (e: Event) => {
    e.preventDefault();
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
        var key = Object.keys(result)[0];
        if (result.code !== undefined && result.message !== undefined) {
          that.storeQuery(that.state.query);
          // This is the case in which user sends a mutation. We display the response from server.
          that.setState({
            resType: 'success-res',
            response: JSON.stringify(result, null, 2)
          })
        } else if (key !== undefined) {
          that.storeQuery(that.state.query);
          // We got the result for a query.
          let response: Object = result[key]
          that.setState({
            'latency': result.server_latency.total
          });
          var startTime = new Date();

          setTimeout(function() {
          // We process all the nodes and edges in the response in background and
          // store the structure in globalNodeSet and globalEdgeSet. We can use this
          // later when we do expansion of nodes.
          let graph = processGraph(response, key, -1);
            globalNodeSet = new vis.DataSet(graph[0])
            globalEdgeSet = new vis.DataSet(graph[1])
            that.setState({
              nodes: graph[0].length,
              relations: graph[1].length
            });
          }, 1000)

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
          var timeTaken = (endTime.getTime() - startTime.getTime()) / 1000;
          let render: string = '';
          if (timeTaken > 1) {
            render = timeTaken.toFixed(3) + 's';
          } else {
            render = (timeTaken - Math.floor(timeTaken)) * 1000 + 'ms';
          }
          that.setState({
            'rendering': render,
            'resType': 'success-res'
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
      var err = (error.response && error.response.text()) || error.message
      return err
    }).then(function(errorMsg) {
      if (errorMsg !== undefined) {
        that.setState({
          response: errorMsg,
          resType: 'error-res'
        })
      }
    })
  }

  enterFullScreen = (e: Event) => {
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
  }

  render = () => {
    var graphClass = classNames({ 'graph-s': true, 'fullscreen': this.state.graph === 'fullscreen' }, { 'App-graph': this.state.graph !== 'fullscreen' }, { 'error-res': this.state.resType === 'error-res' }, { 'success-res': this.state.resType === 'success-res' }, { 'hourglass': this.state.resType === 'hourglass' })
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
                <div className="App-editor">
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
                <div className="App-tip">Tips:<br/>
                  Ctrl + Enter to execute the query.<br/>
                  Ctrl + Up/Down arrow key to see previously run queries.
                </div>
                <div style={{marginTop: '20px', width: '100%', marginBottom: '100px'}}>
                  <span style={{marginLeft: '10px'}}><b>Previous Queries</b></span>
                  <div style={{height: '500px', width: '100%',overflowY: 'scroll', border: '1px solid black', margin: '10px', padding: '10px'}}>
                    {this.state.queries.map(function (query, i) {
                      return <Query text={query} update={this.updateQuery} key={i} unique={i}></Query>;
                    },this)}
                  </div>
                </div>
              </div>
            <div className="col-sm-7">
              <label> Response </label>
                            {
                screenfull.enabled  &&
              <div className="pull-right App-fullscreen" onClick={this.enterFullScreen}>
                <span style={{fontSize: '20px',padding:'5px'}} className="glyphicon glyphicon-glyphicon glyphicon-resize-full">
                </span>
              </div>
              }
              <div style={{width: '100%', height: '100%', padding: '5px'}} id="response">
              <div className={this.state.graphHeight}>
              <div id="graph" className={graphClass}>{this.state.response}</div>
              </div>
              <Stats rendering={this.state.rendering} latency={this.state.latency} class="hidden-xs"></Stats>
              <div>Nodes: {this.state.nodes}, Edges: {this.state.relations}</div>
              <div style={{height:'auto'}}>{this.state.partial === true ? 'We have only loaded a subset of the graph. Double click on a leaf node to expand its child nodes.': ''}</div>
              <div id="properties" style={{marginTop: '10px'}}>Current Node:<div className="App-properties" title={this.state.currentNode}><em><pre>{JSON.stringify(JSON.parse(this.state.currentNode), null, 2)}</pre></em></div>
              </div>
              </div>

            </div>
            <Stats rendering={this.state.rendering} latency={this.state.latency} class="visible-xs"></Stats>
            </div>
            </div>
            <div className="row">
              <div className="col-sm-12">

            </div>
            </div>
          </div> </div>
    );
  }
}

export default App;

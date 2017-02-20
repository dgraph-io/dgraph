// @flow

import React from 'react';

import vis from 'vis';
import screenfull from 'screenfull';
import classNames from 'classnames';
import randomColor from 'randomcolor';

import NavBar from './Navbar';
import Stats from './Stats';
import Query from './Query';
import Label from './Label';

import '../assets/css/App.css'
require('codemirror/addon/hint/show-hint.css');

type Edge = {| id: string, from: string, to: string, arrows: string, label: string, title: string |}
type Node = {| id: string, label: string, title: string, group: string, value: number |}

// Visjs network
var network,
  globalNodeSet,
  globalEdgeSet;

// TODO - Move these three functions to a helper.
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

type Group = {| color: string, label: string |};
type GroupMap = {[key: string]: Group};

// Picked up from http://graphicdesign.stackexchange.com/questions/3682/where-can-i-find-a-large-palette-set-of-contrasting-colors-for-coloring-many-d

var initialRandomColors = ["#47c0ee", "#8dd593", "#f6c4e1", "#8595e1" , "#f79cd4", "#bec1d4",
 "#b5bbe3","#7d87b9", "#e07b91", "#11c638", "#f0b98d", "#4a6fe3"];

var randomColors = [];

function getRandomColor() {
  if (randomColors.length === 0) {
    return randomColor()
  }

  let color = randomColors[0]
  randomColors.splice(0,1)
  return color
}

function checkAndAssign(groups, pred, l, edgeLabels) {
  // This label hasn't been allocated yet.
  groups[pred] = {
    label: l,
    color: getRandomColor()
  }
  edgeLabels[l] = true;
}

// This function shortens and calculates the label for a predicate.
function getGroupProperties(pred: string, edgeLabels: MapOfBooleans,
 groups: GroupMap): Group {
  var prop = groups[pred]
  if (prop !== undefined) {
    // We have already calculated the label for this predicate.
    return prop
  }

  let l
  let dotIdx = pred.indexOf(".")
  if(dotIdx !== -1 && dotIdx !== 0 && dotIdx !== pred.length -1) {
    l = pred[0] + pred[dotIdx+1]
    checkAndAssign(groups,pred, l, edgeLabels)
    return groups[pred]
  }

  for (var i = 1; i <= pred.length; i++) {
    l = pred.substr(0, i)
    // If the first character is not an alphabet we just continue.
    // This saves us from selecting ~ in case of reverse indexed preds.
    if (l.length === 1  && l.toLowerCase() == l.toUpperCase()) {
      continue
    }
    if (edgeLabels[l] === undefined) {
      checkAndAssign(groups,pred, l, edgeLabels)
      return groups[pred]
    }
    // If it has already been allocated, then we increase the substring length and look again.
  }

  groups[pred] = {
    label: pred,
    color: getRandomColor()
  }
  edgeLabels[pred] = true;
  return groups[pred]
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


// Stores the map of a label to boolean (only true values are stored).
// This helps quickly find if a label has already been assigned.
var groups : GroupMap = {};

function processGraph(response: Object, root: string, maxNodes: number) {
  let nodesStack: Array <ResponseNode> = [],
    // Contains map of a lable to its shortform thats displayed.
    predLabel: MapOfStrings = {},
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

    let props = getGroupProperties(obj.src.pred, predLabel, groups)
    if (!uidMap[properties["_uid_"]]) {
      uidMap[properties["_uid_"]] = true
      if (hasProperties(properties) || hasChildNodes) {
        var n: Node = {
          id: properties["_uid_"],
          label: getLabel(properties),
          title: JSON.stringify(properties, null, 2),
          group: obj.src.pred,
          color: props.color,
        }
		    nodes.push(n)
      }
    }

    if (obj.src.id !== "") {
      var e: Edge = {
        id: [obj.src.id, properties["_uid_"]].join("-"),
        from: obj.src.id,
        to: properties["_uid_"],
        title: obj.src.pred,
        label: props.label,
        color: props.color,
        arrows: 'to'
      }
      edges.push(e)
    }
  }

  var plotAxis = []
  for(let pred in groups) {
    plotAxis.push({"label": groups[pred]["label"], "pred": pred, "color": groups[pred]["color"]});
  }

  return [nodes, edges, plotAxis];
}

var doubleClickTime = 0;
var threshold = 200;

function doOnClick(params) {
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
        bindToWindow: false,
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

  // container

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
        let allNodes = adjacentNodeIds.slice();
        let allEdges = outgoingEdges.map(function(edge) {
          return edge.id
        });

        while(adjacentNodeIds.length > 0) {
          let node = adjacentNodeIds.pop()
          let connectedEdges = data.edges.get({
            filter: function(edge) {
              return edge.from === node
            }
          })

          let connectedNodes = connectedEdges.map(function(edge) {
            return edge.to
          })

          allNodes = allNodes.concat(connectedNodes)
          allEdges = allEdges.concat(connectedEdges)
          adjacentNodeIds = adjacentNodeIds.concat(connectedNodes)
        }

        data.nodes.remove(allNodes);
        data.edges.remove(allEdges);
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

  window.onresize = function() { network && network.fit(); }
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
    error["response"] = response
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

function getRootKey(response) {
  let keys = Object.keys(response)
  for(let i = 0; i < keys.length; i++) {
    if(keys[i] != "server_latency" && keys[i] != "uids") {
      return keys[i]
    }
  }
  return ""
}

type QueryTs = {|
  text: string,
  lastRun: number
|}

type State = {
  selectedNode: boolean,
  partial: boolean,
  // queryIndex: number,
  query: string,
  queries: Array <QueryTs>,
  lastQuery: string,
  response: string,
  latency: string,
  rendering: string,
  resType: string,
  currentNode: string,
  nodes: number,
  relations: number,
  graph: string,
  graphHeight: string,
  plotAxis: Array <Object>
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
      // queryIndex: response[0],
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
      plotAxis: []
    };
  }

  updateQuery = (e: Event) => {
    e.preventDefault();
    this.editor.setValue(e.target.dataset.query);
    if(e.target instanceof HTMLElement){
        this.setState({
          query: e.target.dataset.query,
          rendering: '',
          latency: '',
          nodes: 0,
          relations: 0,
          selectedNode: false,
          partial: false,
          currentNode: '{}',
          plotAxis: []
        });
    }
    window.scrollTo(0, 0);
    // this.refs.code.editor.focus();
    // this.refs.code.editor.navigateFileEnd();

    network && network.destroy();
  }

  // Handler which listens to changes on Codemirror.
  queryChange = () => {
    this.setState({ query: this.editor.getValue() });
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
      partial: false,
      lastQuery: this.state.query,
      plotAxis: []
    }
  }

  storeQuery = (query: string) => {
    let queries: Array <QueryTs> = JSON.parse(localStorage.getItem("queries") || '[]');

    query = query.trim();
    queries.forEach(function (q, idx) {
      if (q.text ===  query) {
        queries.splice(idx,1)
      }
    })

    let qu: QueryTs = {text: query, lastRun: Date.now()}
    queries.unshift(qu);

    // (queries.length > 20) && (queries = queries.splice(0, 20))

    this.setState({
      // queryIndex: queries.length - 1,
      queries: queries
    })
    localStorage.setItem("queries", JSON.stringify(queries));
  }

  lastQuery = () => {
    let queries: Array <QueryTs> = JSON.parse(localStorage.getItem("queries") || '[]')
    if (queries.length === 0) {
      return [-1, "", []]
    }

    // We changed the API to hold array of objects instead of strings, so lets clear their localStorage.
    if(queries.length !== 0 && typeof(queries[0]) === "string") {
      let newQueries = [];
      let twoDaysAgo = new Date();
      twoDaysAgo.setDate(twoDaysAgo.getDate() -2);

      for(let i=0; i< queries.length;i++) {
        newQueries.push({
          text: queries[i],
          lastRun: twoDaysAgo
        })
      }
      localStorage.setItem("queries", JSON.stringify(newQueries));
      return [0, newQueries[0].text, newQueries]
    }
    // This means queries has atleast one element.

    return [0, queries[0].text, queries]
  }

  runQuery = (e: Event) => {
    e.preventDefault();
    // Resetting state
    network && network.destroy();
    this.setState(this.resetState());
    randomColors = initialRandomColors.slice();
    groups = {};

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
        var key = getRootKey(result)
        if(key === "") {
          return;
        }
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
            latency: result.server_latency.total,
            resType: ''
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
                relations: graph[1].length,
                plotAxis: graph[2]
              });
          }, 200)

          // We call procesGraph with a 20 node limit and calculate the whole dataset in
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
            render = timeTaken.toFixed(1) + 's';
          } else {
            render = (timeTaken - Math.floor(timeTaken)) * 1000 + 'ms';
          }
          that.setState({
            'rendering': render,
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
                <div className="App-editor" ref={editor => {this._editor = editor;}}/>
                <div style={{marginTop: '10px', width: '100%', marginBottom: '100px'}}>
                  <span><b>Previous Queries</b></span>
                  <table style={{ width: '100%', border: '1px solid black', margin: '15px 0px', padding: '0px 5px 5px 5px'}}>
                    <tbody style={{height: '500px',overflowY: 'scroll', display: 'block'}}>
                    {this.state.queries.map(function (query, i) {
                      return <Query text={query.text} update={this.updateQuery} key={i} lastRun={query.lastRun} unique={i}></Query>;
                    },this)}
                    </tbody>
                  </table>
                </div>
              </div>
            <div className="col-sm-7">
              <label style={{marginLeft: '5px'}}> Response </label>
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
              <div style={{padding: '5px', 'borderWidth': '0px 1px 1px 1px ', 'borderStyle': 'solid', 'borderColor': 'gray',
              textAlign: 'right', margin: '0px'}}>
                <div style={{marginRight: '10px',marginLeft: 'auto'}}>
                {this.state.plotAxis.map(function(label, i) {
                  return <Label key={i} color={label.color} pred={label.pred} label={label.label}></Label>
                }, this)}
                </div>
              </div>
              <div style={{fontSize: '12px'}}>
                <Stats rendering={this.state.rendering} latency={this.state.latency} class="hidden-xs"></Stats>
                <div>Nodes: {this.state.nodes}, Edges: {this.state.relations}</div>
                <div style={{height:'auto'}}>{this.state.partial === true ? 'We have only loaded a subset of the graph. Double click on a leaf node to expand its child nodes.': ''}</div>
                <div id="properties" style={{marginTop: '5px'}}>Current Node:<div className="App-properties" title={this.state.currentNode}>
                <em><pre style={{fontSize: '10px'}}>{JSON.stringify(JSON.parse(this.state.currentNode), null, 2)}</pre></em></div>
              </div>
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

  componentDidMount = () => {
    const CodeMirror = require('codemirror');
    require('codemirror/addon/hint/show-hint');
    require('codemirror/addon/comment/comment');
    require('codemirror/addon/edit/matchbrackets');
    require('codemirror/addon/edit/closebrackets');
    require('codemirror/addon/fold/foldcode');
    require('codemirror/addon/fold/foldgutter');
    require('codemirror/addon/fold/brace-fold');
    require('codemirror/addon/lint/lint');
    require('codemirror/keymap/sublime');
    require('codemirror-graphql/hint');
    require('codemirror-graphql/lint');
    require('codemirror-graphql/info');
    require('codemirror-graphql/jump');
    require('codemirror-graphql/mode');

    let keywords = [];
    timeout(1000, fetch('http://localhost:8080/ui/keywords', {
        method: 'GET',
        mode: 'cors',
      }).then(checkStatus)
      .then(parseJSON)
      .then(function(result) {
        keywords = result.keywords.map(function(kw) {
          return kw.name
        })
      })).catch(function(error) {
      console.log(error.stack)
      console.warn("In catch: Error while trying to fetch list of keywords", error)
      return error
    }).then(function(errorMsg) {
      if(errorMsg !== undefined) {
        console.warn("Error while trying to fetch list of keywords", errorMsg)
      }
    })

    this.editor = CodeMirror(this._editor, {
      value: this.state.query,
      lineNumbers: true,
      tabSize: 2,
      lineWrapping: true,
      mode: 'graphql',
      theme: 'graphiql',
      keyMap: 'sublime',
      autoCloseBrackets: true,
      completeSingle: false,
      showCursorWhenSelecting: true,
      foldGutter: true,
      gutters: [ 'CodeMirror-linenumbers', 'CodeMirror-foldgutter' ],
      extraKeys: {
        'Ctrl-Space': (cm) =>  { CodeMirror.commands.autocomplete(cm)},
        'Cmd-Space': (cm) =>  { CodeMirror.commands.autocomplete(cm)},
        'Cmd-Enter': () => {
            this.runQuery(new Event(''));
        },
        'Ctrl-Enter': () => {
            this.runQuery(new Event(''));
        },
      },
      autofocus: true
    });

    this.editor.setCursor(this.editor.lineCount(), 0);

    this.editor.on('change', this.queryChange)

    CodeMirror.registerHelper("hint", "fromList", function(cm, options) {
      var cur = cm.getCursor(), token = cm.getTokenAt(cur);

      // This is so that we automatically have a space before (, so that auto-
      // complete inside braces works. Otherwise it doesn't work for
      // director.film(orderasc: release_date).
      let openBrac = token.string.indexOf("(")
      if(openBrac !== -1 && token.string[openBrac-1] != undefined &&
        token.string[openBrac-1] != " "){
        let oldString = token.string.substr(openBrac)
        cm.replaceRange(" ", {line: cur.line, ch: token.start + openBrac})
      }

      var to = CodeMirror.Pos(cur.line, token.end);
      if (token.string) {
        var term = token.string, from = CodeMirror.Pos(cur.line, token.start);
      } else {
        var term = "", from = to;
      }

      // So that we don't autosuggest for anyof/allof filter values which
      // would be inside quotes.
      if(term.length > 0 && term[0] === '"') {
        return {list: [], from: from, to: to}
      }

      // TODO - This is a hack because Graphiql mode considers . as an invalidchar.
      // Ideally we should write our own mode which allows . in predicate.
      if(token.type === "invalidchar" && token.state.prevState !== undefined &&
        token.state.prevState.kind === "Field") {
          term = token.state.prevState.name + token.string
          from.ch = from.ch - token.state.prevState.name.length
      }

      // because Codemirror strips the @ from a directive.
      if(token.state.kind === "Directive") {
        term = "@" + term
        from.ch -= 1
      }

      term = term.toLowerCase();

      var found = [];
      for (var i = 0; i < options.words.length; i++) {
        var word = options.words[i];
        if (term.length > 0 && word.startsWith(term)) {
          found.push(word);
        }
      }

      if (found.length) return {list: found, from: from, to: to};
    });

    CodeMirror.commands.autocomplete = function (cm) {
      CodeMirror.showHint(cm, CodeMirror.hint.fromList, {
        completeSingle: false,
        words: keywords
      })
    }

 this.editor.on("keydown", function (cm, event) {
    const code = event.keyCode;

    if (!event.ctrlKey && code >= 65 && code <= 90) {
      CodeMirror.commands.autocomplete(cm);
    }
    });
  }
}

export default App;
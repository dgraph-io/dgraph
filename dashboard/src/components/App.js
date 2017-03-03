// @flow

import React from "react";

import screenfull from "screenfull";
import randomColor from "randomcolor";
import uuid from "uuid";

import NavBar from "./Navbar";
import Stats from "./Stats";
import PreviousQuery from "./PreviousQuery";
import Editor from "./Editor";
import Response from "./Response";
import { getNodeLabel, isShortestPath, aggregationPrefix } from "./Helpers";

import "../assets/css/App.css";

type Edge = {|
  from: string,
  to: string,
  arrows: string,
  label: string,
  title: string,
|};

type Node = {|
  id: string,
  label: string,
  title: string,
  group: string,
  color: string,
|};

type MapOfStrings = { [key: string]: string };
type MapOfBooleans = { [key: string]: boolean };

type Group = {| color: string, label: string |};
type GroupMap = { [key: string]: Group };

type Src = {| id: string, pred: string |};
type ResponseNode = {| node: Object, src: Src |};

// Stores the map of a label to boolean (only true values are stored).
// This helps quickly find if a label has already been assigned.
var groups: GroupMap = {};

function hasChildren(node: Object): boolean {
  for (var prop in node) {
    if (Array.isArray(node[prop])) {
      return true;
    }
  }
  return false;
}

// Picked up from http://graphicdesign.stackexchange.com/questions/3682/where-can-i-find-a-large-palette-set-of-contrasting-colors-for-coloring-many-d
var initialRandomColors = [
  "#47c0ee",
  "#8dd593",
  "#f6c4e1",
  "#8595e1",
  "#f0b98d",
  "#f79cd4",
  "#bec1d4",
  "#11c638",
  "#b5bbe3",
  "#7d87b9",
  "#e07b91",
  "#4a6fe3",
];

var randomColors = [];

function getRandomColor() {
  if (randomColors.length === 0) {
    return randomColor();
  }

  let color = randomColors[0];
  randomColors.splice(0, 1);
  return color;
}

function checkAndAssign(groups, pred, l, edgeLabels) {
  // This label hasn't been allocated yet.
  groups[pred] = {
    label: l,
    color: getRandomColor(),
  };
  edgeLabels[l] = true;
}

// This function shortens and calculates the label for a predicate.
function getGroupProperties(
  pred: string,
  edgeLabels: MapOfBooleans,
  groups: GroupMap,
): Group {
  var prop = groups[pred];
  if (prop !== undefined) {
    // We have already calculated the label for this predicate.
    return prop;
  }

  let l;
  let dotIdx = pred.indexOf(".");
  if (dotIdx !== -1 && dotIdx !== 0 && dotIdx !== pred.length - 1) {
    l = pred[0] + pred[dotIdx + 1];
    checkAndAssign(groups, pred, l, edgeLabels);
    return groups[pred];
  }

  for (var i = 1; i <= pred.length; i++) {
    l = pred.substr(0, i);
    // If the first character is not an alphabet we just continue.
    // This saves us from selecting ~ in case of reverse indexed preds.
    if (l.length === 1 && l.toLowerCase() === l.toUpperCase()) {
      continue;
    }
    if (edgeLabels[l] === undefined) {
      checkAndAssign(groups, pred, l, edgeLabels);
      return groups[pred];
    }
    // If it has already been allocated, then we increase the substring length and look again.
  }

  groups[pred] = {
    label: pred,
    color: getRandomColor(),
  };
  edgeLabels[pred] = true;
  return groups[pred];
}

function hasProperties(props: Object): boolean {
  // Each node will have a _uid_. We check if it has other properties.
  return Object.keys(props).length !== 1;
}

function processGraph(response: Object, treeView: boolean, query: string) {
  let nodesStack: Array<ResponseNode> = [],
    // Contains map of a lable to its shortform thats displayed.
    predLabel: MapOfStrings = {},
    // Map of whether a Node with an Uid has already been created. This helps
    // us avoid creating duplicating nodes while parsing the JSON structure
    // which is a tree.
    uidMap: MapOfBooleans = {},
    nodes: Array<Node> = [],
    edges: Array<Edge> = [],
    emptyNode: ResponseNode = {
      node: {},
      src: {
        id: "",
        pred: "empty",
      },
    },
    someNodeHasChildren: boolean = false,
    ignoredChildren: Array<ResponseNode> = [],
    shortestPath: boolean = isShortestPath(query),
    // We store the indexes corresponding to what we show at first render here.
    // That we can only do one traversal.
    nodesIndex,
    maxNodes = 2,
    edgesIndex;

  for (var root in response) {
    if (!response.hasOwnProperty(root)) {
      continue;
    }

    someNodeHasChildren = false;
    ignoredChildren = [];
    if (root !== "server_latency" && root !== "uids") {
      let block = response[root];
      for (let i = 0; i < block.length; i++) {
        let rn: ResponseNode = {
          node: block[i],
          src: {
            id: "",
            pred: root,
          },
        };
        if (hasChildren(block[i])) {
          someNodeHasChildren = true;
          nodesStack.push(rn);
        } else {
          ignoredChildren.push(rn);
        }

        if (shortestPath && i - 1 >= 0) {
          // Fo shortest path, we create edges between the root nodes.
          edges.push({
            to: block[i]["_uid_"],
            from: block[i - 1]["_uid_"],
            arrows: "to",
            label: "p",
            title: "{}",
          });
        }
      }

      // If no node has children or its a shortest path query, then we add root
      // level nodes to the view.
      if (!someNodeHasChildren || shortestPath) {
        nodesStack.push.apply(nodesStack, ignoredChildren);
      }
    }
  }

  // We push an empty node after all the children. This would help us know when
  // we have traversed all nodes at a level.
  nodesStack.push(emptyNode);

  while (nodesStack.length > 0) {
    let obj = nodesStack.shift();
    // Check if this is an empty node.
    if (Object.keys(obj.node).length === 0 && obj.src.pred === "empty") {
      if (nodesStack.length === 0) {
        break;
      } else if (
        nodes.length >= maxNodes &&
        nodesIndex === undefined &&
        edgesIndex === undefined
      ) {
        nodesIndex = nodes.length;
        edgesIndex = edges.length;
      } else {
        nodesStack.push(emptyNode);
      }
      continue;
    }

    let properties: MapOfStrings = {
      attrs: {},
      facets: {},
    },
      hasChildNodes: boolean = false,
      id: string,
      edgeAttributes = {
        facets: {},
      },
      uid: string;

    uid = obj.node["_uid_"] === undefined ? uuid() : obj.node["_uid_"];
    id = treeView
      ? // For tree view, the id is the join of ids of this node
        // with all its ancestors. That would make it unique.
        [obj.src.id, uid].filter(val => val).join("-")
      : uid;

    for (let prop in obj.node) {
      if (!obj.node.hasOwnProperty(prop)) {
        continue;
      }

      // We can have a key-val pair, another array or an object here (in case of facets)
      let val = obj.node[prop];
      if (Array.isArray(val)) {
        hasChildNodes = true;
        let arr = val, xposition = 1;
        for (let j = 0; j < arr.length; j++) {
          arr[j]["x"] = xposition++;
          nodesStack.push({
            node: arr[j],
            src: {
              pred: prop,
              id: id,
            },
          });
        }
      } else if (typeof val === "object") {
        if (prop === "@facets") {
          // lets handle @facets between uids here.
          for (let pred in val) {
            if (!val.hasOwnProperty(pred)) {
              continue;
            }

            // pred could either be _ or other predicates. If its a predicate it could have
            // multiple k-v pairs.
            if (pred === "_") {
              edgeAttributes["facets"] = val["_"];
            } else {
              let predFacets = val[pred];
              for (let f in predFacets) {
                if (!predFacets.hasOwnProperty(f)) {
                  continue;
                }

                properties["facets"][`${pred}[${f}]`] = predFacets[f];
              }
            }
          }
        }
      } else {
        properties["attrs"][prop] = val;
      }
    }

    if (!hasProperties(obj) && !hasChildNodes) {
      continue;
    }

    let nodeAttrs = properties["attrs"];
    let aggrTerm = aggregationPrefix(nodeAttrs);
    let name = aggrTerm !== "" ? aggrTerm : obj.src.pred;

    let props = getGroupProperties(name, predLabel, groups);
    let x = nodeAttrs["x"];
    delete nodeAttrs["x"];

    let n: Node = {
      id: id,
      x: x,
      label: getNodeLabel(nodeAttrs),
      title: JSON.stringify(properties),
      color: props.color,
      group: obj.src.pred,
    };

    if (treeView) {
      // For tree view, we push duplicate nodes too.
      nodes.push(n);
    } else {
      if (!uidMap[id]) {
        uidMap[id] = true;
        nodes.push(n);
      }
    }

    if (obj.src.id === "") {
      continue;
    }

    var e: Edge = {
      from: obj.src.id,
      to: id,
      title: JSON.stringify(edgeAttributes),
      label: props.label,
      color: props.color,
      arrows: "to",
    };
    edges.push(e);
  }

  var plotAxis = [];
  for (let pred in groups) {
    if (!groups.hasOwnProperty(pred)) {
      continue;
    }

    plotAxis.push({
      label: groups[pred]["label"],
      pred: pred,
      color: groups[pred]["color"],
    });
  }

  return [nodes, edges, plotAxis, nodesIndex, edgesIndex];
}

type QueryTs = {|
  text: string,
  lastRun: number,
|};

type State = {
  selectedNode: boolean,
  lastQuery: string,
  queries: Array<QueryTs>,
  lastQuery: string,
  response: string,
  latency: string,
  rendering: string,
  resType: string,
  graph: string,
  graphHeight: string,
  plotAxis: Array<Object>,
};

type Props = {};

class App extends React.Component {
  state: State;

  constructor(props: Props) {
    super(props);
    let response = this.lastQuery();

    this.state = {
      selectedNode: false,
      lastQuery: response[1],
      // We store the queries run in state, so that they can be displayed
      // to the user.
      queries: response[2],
      response: "",
      result: {},
      latency: "",
      rendering: "",
      resType: "",
      graph: "",
      graphHeight: "Graph-fixed-height",
      plotAxis: [],

      nodes: [],
      edges: [],
      allNodes: [],
      allEdges: [],
      treeView: false,
    };
  }

  updateQuery = (e: Event) => {
    e.preventDefault();
    if (e.target instanceof HTMLElement) {
      this.setState({
        lastQuery: e.target.dataset.query,
        rendering: "",
        latency: "",
        selectedNode: false,
        partial: false,
        plotAxis: [],
        result: {},
        nodes: [],
        edges: [],
        allNodes: [],
        allEdges: [],
        treeView: false,
      });
    }
    window.scrollTo(0, 0);
  };

  // Handler which is used to update lastQuery by Editor component..
  queryChange = query => {
    this.setState({ lastQuery: query });
  };

  resetState = () => {
    return {
      response: "",
      selectedNode: false,
      latency: "",
      rendering: "",
      resType: "hourglass",
      plotAxis: [],
      result: {},
      allNodes: [],
      allEdges: [],
      nodes: [],
      edges: [],
      treeView: false,
    };
  };

  storeQuery = () => {
    let queries: Array<QueryTs> = JSON.parse(
      localStorage.getItem("queries") || "[]",
    );

    let query = this.state.lastQuery.trim();
    queries.forEach(function(q, idx) {
      if (q.text === query) {
        queries.splice(idx, 1);
      }
    });

    let qu: QueryTs = { text: query, lastRun: Date.now() };
    queries.unshift(qu);

    this.setState({
      queries: queries,
    });
    localStorage.setItem("queries", JSON.stringify(queries));
  };

  lastQuery = () => {
    let queries: Array<QueryTs> = JSON.parse(
      localStorage.getItem("queries") || "[]",
    );
    if (queries.length === 0) {
      return [-1, "", []];
    }

    // We changed the API to hold array of objects instead of strings, so lets clear their localStorage.
    if (queries.length !== 0 && typeof queries[0] === "string") {
      let newQueries = [];
      let twoDaysAgo = new Date();
      twoDaysAgo.setDate(twoDaysAgo.getDate() - 2);

      for (let i = 0; i < queries.length; i++) {
        newQueries.push({
          text: queries[i],
          lastRun: twoDaysAgo,
        });
      }
      localStorage.setItem("queries", JSON.stringify(newQueries));
      return [0, newQueries[0].text, newQueries];
    }
    // This means queries has atleast one element.

    return [0, queries[0].text, queries];
  };

  enterFullScreen = (e: Event) => {
    e.preventDefault();
    document.addEventListener(screenfull.raw.fullscreenchange, () => {
      if (!screenfull.isFullscreen) {
        this.setState({
          graph: "",
          graphHeight: "Graph-fixed-height",
        });
      } else {
        // In full screen mode, we display the properties as a tooltip.
        this.setState({
          graph: "fullscreen",
          graphHeight: "Graph-full-height",
        });
      }
    });
    screenfull.request(document.getElementById("response"));
  };

  resetStateOnQuery = () => {
    this.setState(this.resetState());
    randomColors = initialRandomColors.slice();
    groups = {};
  };

  renderGraph = (result, treeView) => {
    let startTime = new Date(), that = this;

    let [nodes, edges, labels, nodesIdx, edgesIdx] = processGraph(
      result,
      treeView,
      this.state.lastQuery,
    );

    that.setState({
      plotAxis: labels,
      allNodes: nodes,
      allEdges: edges,
      nodes: nodes.slice(0, nodesIdx),
      edges: edges.slice(0, edgesIdx),
      treeView: treeView,
    });

    let endTime = new Date(),
      timeTaken = (endTime.getTime() - startTime.getTime()) / 1000,
      render = "";

    if (timeTaken > 1) {
      render = timeTaken.toFixed(1) + "s";
    } else {
      render = (timeTaken - Math.floor(timeTaken)) * 1000 + "ms";
    }

    that.setState({
      latency: result.server_latency.total,
      resType: "",
      result: result,
      rendering: render,
    });
  };

  renderResText = (type, text) => {
    this.setState({
      resType: type,
      response: text,
    });
  };

  render = () => {
    return (
      <div>
        <NavBar />
        <div className="container-fluid">
          <div className="row justify-content-md-center">
            <div className="col-sm-12">
              <div className="col-sm-5">
                <Editor
                  query={this.state.lastQuery}
                  updateQuery={this.queryChange}
                  storeLastQuery={this.storeQuery}
                  resetState={this.resetStateOnQuery}
                  renderGraph={this.renderGraph}
                  renderResText={this.renderResText}
                />

                <div className="App-prev-queries">
                  <span><b>Previous Queries</b></span>
                  <table className="App-prev-queries-table">
                    <tbody className="App-prev-queries-tbody">
                      {this.state.queries.map(
                        function(query, i) {
                          return (
                            <PreviousQuery
                              text={query.text}
                              update={this.updateQuery}
                              key={i}
                              lastRun={query.lastRun}
                              unique={i}
                            />
                          );
                        },
                        this,
                      )}
                    </tbody>
                  </table>
                </div>
              </div>
              <div className="col-sm-7">
                <label style={{ marginLeft: "5px" }}> Response </label>
                {screenfull.enabled &&
                  <div
                    className="pull-right App-fullscreen"
                    onClick={this.enterFullScreen}
                  >
                    <span
                      className="App-fs-icon glyphicon glyphicon-glyphicon glyphicon-resize-full"
                    />
                  </div>}
                <Response
                  graph={this.state.graph}
                  resType={this.state.resType}
                  graphHeight={this.state.graphHeight}
                  response={this.state.response}
                  result={this.state.result}
                  plotAxis={this.state.plotAxis}
                  rendering={this.state.rendering}
                  latency={this.state.latency}
                  partial={this.state.partial}
                  nodes={this.state.nodes}
                  edges={this.state.edges}
                  allNodes={this.state.allNodes}
                  allEdges={this.state.allEdges}
                  treeView={this.state.treeView}
                  renderGraph={this.renderGraph}
                />
              </div>
            </div>
          </div>
          <div className="row">
            <div className="col-sm-12" />
          </div>
        </div>{" "}
      </div>
    );
  };
}

export default App;

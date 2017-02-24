// @flow

import React from "react";

import screenfull from "screenfull";
import randomColor from "randomcolor";

import NavBar from "./Navbar";
import Stats from "./Stats";
import PreviousQuery from "./PreviousQuery";
import Editor from "./Editor";
import Response from "./Response";
import { getNodeLabel } from "./Helpers";

import "../assets/css/App.css";

type Edge = {|
  id: string,
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
  value: number,
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
  "#f79cd4",
  "#f0b98d",
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

function processGraph(response: Object, maxNodes: number) {
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
    ignoredChildren: Array<ResponseNode> = [];

  for (var root in response) {
    if (!response.hasOwnProperty(root)) {
      continue;
    }

    someNodeHasChildren = false;
    ignoredChildren = [];
    if (root !== "server_latency" && root !== "uids") {
      let block = response[root];
      for (let i = 0; i < block.length; i++) {
        let n: ResponseNode = {
          node: block[i],
          src: {
            id: "",
            pred: root,
          },
        };
        if (hasChildren(block[i])) {
          someNodeHasChildren = true;
          nodesStack.push(n);
        } else {
          ignoredChildren.push(n);
        }
      }

      // Lets put in the root nodes which have any children here.
      if (!someNodeHasChildren) {
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
      // We break out if we have reached the max node limit.
      let maxNodesElapsed = maxNodes !== -1 && nodes.length > maxNodes;
      if (nodesStack.length === 0 || maxNodesElapsed) {
        break;
      } else {
        nodesStack.push(emptyNode);
        continue;
      }
    }

    let properties: MapOfStrings = {}, hasChildNodes: boolean = false;

    for (let prop in obj.node) {
      // If its just a value, then we store it in properties for this node.
      if (!Array.isArray(obj.node[prop])) {
        properties[prop] = obj.node[prop];
      } else {
        hasChildNodes = true;
        let arr = obj.node[prop];
        for (let i = 0; i < arr.length; i++) {
          nodesStack.push({
            node: arr[i],
            src: {
              pred: prop,
              id: obj.node["_uid_"],
            },
          });
        }
      }
    }

    let props = getGroupProperties(obj.src.pred, predLabel, groups);
    if (!uidMap[properties["_uid_"]]) {
      uidMap[properties["_uid_"]] = true;
      if (hasProperties(properties) || hasChildNodes) {
        var n: Node = {
          id: properties["_uid_"],
          label: getNodeLabel(properties),
          title: JSON.stringify(properties, null, 2),
          group: obj.src.pred,
          color: props.color,
        };
        nodes.push(n);
      }
    }

    if (obj.src.id !== "") {
      var e: Edge = {
        // id: [obj.src.id, properties["_uid_"]].join("-"),
        from: obj.src.id,
        to: properties["_uid_"],
        title: obj.src.pred,
        label: props.label,
        color: props.color,
        arrows: "to",
      };
      edges.push(e);
    }
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

  return [nodes, edges, plotAxis];
}

function renderPartialGraph(result) {
  var graph = processGraph(result, 2), that = this;

  setTimeout(
    function() {
      that.setState({
        nodes: graph[0],
        edges: graph[1],
      });
    },
    1000,
  );
}

type QueryTs = {|
  text: string,
  lastRun: number,
|};

type State = {
  selectedNode: boolean,
  partial: boolean,
  lastQuery: string,
  queries: Array<QueryTs>,
  lastQuery: string,
  response: string,
  latency: string,
  rendering: string,
  resType: string,
  nodes: number,
  relations: number,
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
      partial: false,
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
      graphHeight: "fixed-height",
      plotAxis: [],
      expandText: "Expand",

      nodes: [],
      edges: [],
      allNodes: [],
      allEdges: [],
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
        expandDisabled: true,
        expandText: "Expand",
        result: {},
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
      partial: false,
      plotAxis: [],
      expandDisabled: true,
      expandText: "Expand",
      result: {},
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
          graphHeight: "fixed-height",
        });
      } else {
        // In full screen mode, we display the properties as a tooltip.
        this.setState({
          graph: "fullscreen",
          graphHeight: "full-height",
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

  renderGraph = result => {
    let startTime = new Date(), that = this;

    setTimeout(
      function() {
        // We process all the nodes and edges in the response in background and
        // store the structure in globalNodeSet and globalEdgeSet. We can use this
        // later when we do expansion of nodes.
        let graph = processGraph(result, -1);

        that.setState({
          plotAxis: graph[2],
          allNodes: graph[0],
          allEdges: graph[1],
        });
      },
      200,
    );
    // We call procesGraph with a 5 node limit and calculate the whole dataset in
    // the background.
    renderPartialGraph.bind(that, result)();

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

                <div
                  style={{
                    marginTop: "10px",
                    width: "100%",
                    marginBottom: "100px",
                  }}
                >
                  <span><b>Previous Queries</b></span>
                  <table
                    style={{
                      width: "100%",
                      border: "1px solid black",
                      margin: "15px 0px",
                      padding: "0px 5px 5px 5px",
                    }}
                  >
                    <tbody
                      style={{
                        height: "500px",
                        overflowY: "scroll",
                        display: "block",
                      }}
                    >
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
                      style={{ fontSize: "20px", padding: "5px" }}
                      className="glyphicon glyphicon-glyphicon glyphicon-resize-full"
                    />
                  </div>}
                <Response
                  graph={this.state.graph}
                  resType={this.state.resType}
                  graphHeight={this.state.graphHeight}
                  response={this.state.response}
                  plotAxis={this.state.plotAxis}
                  rendering={this.state.rendering}
                  latency={this.state.latency}
                  expandText={this.state.expandText}
                  partial={this.state.partial}
                  nodes={this.state.nodes}
                  edges={this.state.edges}
                  allNodes={this.state.allNodes}
                  allEdges={this.state.allEdges}
                />
              </div>
              <Stats
                rendering={this.state.rendering}
                latency={this.state.latency}
                class="visible-xs"
              />
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

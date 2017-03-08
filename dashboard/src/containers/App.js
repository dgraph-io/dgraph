// @flow

import React from "react";

import screenfull from "screenfull";
import randomColor from "randomcolor";
import uuid from "uuid";
import { connect } from "react-redux";

import NavBar from "../components/Navbar";
import PreviousQueryList from "./PreviousQueryList";
import Editor from "./Editor";
import Response from "./Response";
// TODO - See if we should move helper somewhere else.
import { getNodeLabel, aggregationPrefix } from "./Helpers";

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

  // TODO - Verify that it works with Redux store.
  // deleteQuery = idx => {
  //   if (idx < 0) {
  //     return;
  //   }

  //   // TODO - Abstract out, get, put delete so that state is updated both for react
  //   // and localStorage. Maybe Redux can help with this?
  //   let q = this.state.queries;
  //   q.splice(idx, 1);
  //   this.setState({
  //     queries: q,
  //   });
  //   let queries = JSON.parse(localStorage.getItem("queries"));
  //   queries.splice(idx, 1);
  //   localStorage.setItem("queries", JSON.stringify(queries));
  // };

  // Handler which is used to update lastQuery by Editor component.
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

  // TODO - Shifted to redux store and redux-persist
  // storeQuery = () => {
  //   let queries: Array<QueryTs> = JSON.parse(
  //     localStorage.getItem("queries") || "[]",
  //   );

  //   let query = this.state.lastQuery.trim();
  //   queries.forEach(function(q, idx) {
  //     if (q.text === query) {
  //       queries.splice(idx, 1);
  //     }
  //   });

  //   let qu: QueryTs = { text: query, lastRun: Date.now() };
  //   queries.unshift(qu);

  //   this.setState({
  //     queries: queries,
  //   });
  //   localStorage.setItem("queries", JSON.stringify(queries));
  // };

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

  // renderResText = (type, text) => {
  //   this.setState({
  //     resType: type,
  //     response: text,
  //   });
  // };

  render = () => {
    return (
      <div>
        <NavBar />
        <div className="container-fluid">
          <div className="row justify-content-md-center">
            <div className="col-sm-12">
              <div className="col-sm-5">
                <Editor query={this.state.lastQuery} />
                {/*
                  updateQuery={this.queryChange}
                  resetState={this.resetStateOnQuery}
                  renderGraph={this.renderGraph}
*/
                }

                {/*
                <PreviousQueryList />
                  queries={this.state.queries}
                  update={this.updateQuery}
                  delete={this.deleteQuery}
                  xs="hidden-xs"
                  */
                }
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
                <Response />
                {/*graph={this.state.graph}
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
                }
                {/*<PreviousQueryList />
                 queries={this.state.queries}
                  update={this.updateQuery}
                  delete={this.deleteQuery}
                  xs="visible-xs-block"
                */
                }
              </div>
            </div>
          </div>
        </div>
      </div>
    );
  };
}

export default App;

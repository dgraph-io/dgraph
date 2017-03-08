// @flow

import React from "react";
import screenfull from "screenfull";

import NavBar from "../components/Navbar";
import PreviousQueryList from "./PreviousQueryList";
import Editor from "./Editor";
import Response from "./Response";

import "../assets/css/App.css";

// TODO - Move these to the appropriate place and run all files through
// Flow.
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

type QueryTs = {|
  text: string,
  lastRun: number,
|};

class App extends React.Component {
  state: State;

  constructor(props: Props) {
    super(props);
    let response = this.lastQuery();

    this.state = {
      lastQuery: response[1],
      graph: "",
      graphHeight: "Graph-fixed-height",
    };
  }

  updateQuery = (e: Event) => {
    e.preventDefault();
    if (e.target instanceof HTMLElement) {
      this.setState({
        lastQuery: e.target.dataset.query,
        partial: false,
      });
    }
    window.scrollTo(0, 0);
  };

  // Handler which is used to update lastQuery by Editor component.
  queryChange = query => {
    this.setState({ lastQuery: query });
  };

  resetState = () => {
    return {
      resType: "hourglass",
    };
  };

  // TODO - Move this to use the redux store too.
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

  // TODO - Fix this. Get states from redux store.
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

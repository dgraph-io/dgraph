import React, { Component } from "react";
import "../assets/css/App.css";

import "brace";
import AceEditor from "react-ace";
import "brace/mode/logiql";
import "brace/theme/github";

class Editor extends Component {
  lastQuery = () => {
    var queries = JSON.parse(localStorage.getItem("queries"));
    if (queries == null) {
      return [undefined, "", []];
    }
    // This means queries has atleast one element.

    return [0, queries[0], queries];
  };

  nextQuery = () => {
    var queries = JSON.parse(localStorage.getItem("queries"));
    if (queries == null) {
      return;
    }

    var idx = this.state.queryIndex;
    if (idx === undefined || idx - 1 < 0) {
      return;
    }
    this.setState({
      query: queries[idx - 1],
      queryIndex: idx - 1,
    });
  };

  previousQuery = () => {
    var queries = JSON.parse(localStorage.getItem("queries"));
    if (queries == null) {
      return;
    }

    var idx = this.state.queryIndex;
    if (idx === undefined || idx + 1 === queries.length) {
      return;
    }
    this.setState({
      queryIndex: idx + 1,
      query: queries[idx + 1],
    });
  };

  runQuery = e => {
    e.preventDefault();
    // Resetting state
    network && network.destroy();
    this.setState(this.resetState());

    var that = this;
    timeout(
      60000,
      fetch("http://localhost:8080/query?debug=true", {
        method: "POST",
        mode: "cors",
        headers: {
          "Content-Type": "text/plain",
        },
        body: this.state.query,
      })
        .then(checkStatus)
        .then(parseJSON)
        .then(function(result) {
          var key = Object.keys(result)[0];
          if (result.code !== undefined && result.message !== undefined) {
            that.storeQuery(that.state.query);
            // This is the case in which user sends a mutation. We display the response from server.
            that.setState({
              resType: "success-res",
              response: JSON.stringify(result, null, 2),
            });
          } else if (key !== undefined) {
            that.storeQuery(that.state.query);
            // We got the result for a query.
            var response = result[key];
            that.setState({
              latency: result.server_latency.total,
            });
            var startTime = new Date();

            setTimeout(
              function() {
                var graph = processGraph(response, key, -1);
                nodeSet = new vis.DataSet(graph[0]);
                edgeSet = new vis.DataSet(graph[1]);
                that.setState({
                  nodes: graph[0].length,
                  relations: graph[1].length,
                });
              },
              1000,
            );

            // We call procesGraph with a 40 node limit and calculate the whole dataset in
            // the background.
            var graph = processGraph(response, key, 20);
            setTimeout(
              (function() {
                that.setState({
                  partial: this.state.nodes > graph[0].length,
                });
              }).bind(that),
              1000,
            );

            renderNetwork.call(that, graph[0], graph[1]);

            var endTime = new Date();
            var timeTaken = ((endTime.getTime() - startTime.getTime()) /
              1000).toFixed(2);
            that.setState({
              rendering: timeTaken + "s",
              resType: "success-res",
            });
          } else {
            console.warn("We shouldn't be here really");
            // We probably didn't get any results.
            that.setState({
              resType: "error-res",
              response: "Your query did not return any results.",
            });
          }
        }),
    )
      .catch(function(error) {
        console.log(error.stack);
        var err = error.response && error.response.text() || error.message;
        return err;
      })
      .then(function(errorMsg) {
        if (errorMsg !== undefined) {
          that.setState({
            response: errorMsg,
            resType: "error-res",
          });
        }
      });
  };

  render() {
    return (
      <div className="App-editor">
        <AceEditor
          mode="logiql"
          theme="github"
          name="editor"
          editorProps={{ $blockScrolling: Infinity }}
          width="100%"
          height="350px"
          fontSize="12px"
          ref="code"
          value={this.props.query}
          focus={true}
          showPrintMargin={false}
          wrapEnabled={true}
          onChange={this.propsqueryChange}
          commands={[
            {
              name: "runQuery",
              bindKey: {
                win: "Ctrl-Enter",
                mac: "Command-Enter",
              },
              exec: (function(editor) {
                this.runQuery(new Event(""));
              }).bind(this),
            },
            {
              name: "previousQuery",
              bindKey: {
                win: "Ctrl-Up",
                mac: "Command-Up",
              },
              exec: (function(editor) {
                this.previousQuery();
              }).bind(this),
            },
            {
              name: "nextQuery",
              bindKey: {
                win: "Ctrl-Down",
                mac: "Command-Down",
              },
              exec: (function(editor) {
                this.nextQuery();
              }).bind(this),
            },
          ]}
        />
      </div>
    );
  }
}

export default Editor;

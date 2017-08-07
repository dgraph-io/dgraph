import React from "react";
import Raven from "raven-js";

import FrameLayout from "./FrameLayout";
import FrameSession from "./FrameSession";
import FrameError from "./FrameError";
import FrameSuccess from "./FrameSuccess";
import FrameLoading from "./FrameLoading";

import { executeQuery, isNotEmpty, getSharedQuery } from "../lib/helpers";
import { processGraph } from "../lib/graph";

class FrameItem extends React.Component {
  constructor(props) {
    super(props);

    this.state = {
      // FIXME: naming could be better. logically data should be called response
      // and vice-versa
      // data is a raw JSON response from Dgraph
      data: null,
      // response is a processed version of data suited to render graph
      response: null,
      executed: false,
      errorMessage: null,
      successMessage: null
    };
  }

  componentDidMount() {
    const { frame } = this.props;
    const { query, share, meta } = frame;

    if (!meta.collapsed && query && query.length > 0) {
      this.executeFrameQuery(query);
    } else if (share && share.length > 0 && !query) {
      this.getAndExecuteSharedQuery(share);
    }
  }

  cleanFrameData = () => {
    this.setState({
      data: null,
      response: null,
      executed: false,
      errorMessage: null,
      successMessage: null
    });
  };

  getAndExecuteSharedQuery = shareId => {
    return getSharedQuery(shareId)
      .then(query => {
        if (!query) {
          this.setState({
            errorMessage: `No query found for the shareId: ${shareId}`,
            executed: true
          });
        } else {
          this.executeFrameQuery(query);
          const { frame, updateFrame } = this.props;
          updateFrame({
            query: query,
            id: frame.id,
            // Lets update share back to empty, because we now have the query.
            share: ""
          })();
        }
      })
      .catch(error => {
        Raven.captureException(error);
        this.setState({
          executed: true,
          data: error,
          errorMessage: error.message
        });
      });
  };

  executeFrameQuery = query => {
    const { frame: { meta }, onUpdateConnectedState } = this.props;

    executeQuery(query)
      .then(res => {
        onUpdateConnectedState(true);

        if (res.errors) {
          // Handle query error responses here.
          this.setState({
            errorMessage: res.errors[0].message,
            data: res,
            executed: true
          });
        } else if (
          res.data &&
          res.data.code !== undefined &&
          res.data.message !== undefined
        ) {
          // This is the case in which user sends a mutation.
          // We display the response from server.
          if (res.data.code.startsWith("Error")) {
            this.setState({
              errorMessage: res.data.message,
              data: res,
              executed: true
            });
          } else {
            this.setState({
              successMessage: res.data.message,
              data: res,
              executed: true
            });
          }
        } else if (isNotEmpty(res.data)) {
          const regexStr = meta.regexStr || "Name";
          const { nodes, edges, labels, nodesIndex, edgesIndex } = processGraph(
            res.data,
            false,
            query,
            regexStr
          );

          const response = {
            plotAxis: labels,
            allNodes: nodes,
            allEdges: edges,
            numNodes: nodes.length,
            numEdges: edges.length,
            nodes: nodes.slice(0, nodesIndex),
            edges: edges.slice(0, edgesIndex),
            treeView: false,
            data: res
          };

          this.setState({ response, executed: true, data: res });
        } else {
          this.setState({
            successMessage: "Your query did not return any results",
            executed: true,
            data: res
          });
        }
      })
      .catch(error => {
        // FIXME: make it DRY. but error.response.text() is async and error.message is sync

        // if no response, it's a network error or client side runtime error
        if (!error.response) {
          // Capture client side error not query execution error from server
          // FIXME: This captures 404
          Raven.captureException(error);
          onUpdateConnectedState(false);

          this.setState({
            errorMessage: `${error.message}: Could not connect to the server`,
            executed: true,
            data: error
          });
        } else {
          error.response.text().then(text => {
            this.setState({ errorMessage: text, executed: true });
          });
        }
      });
  };

  render() {
    const {
      frame,
      onDiscardFrame,
      onSelectQuery,
      collapseAllFrames
    } = this.props;
    const {
      errorMessage,
      successMessage,
      response,
      executed,
      data
    } = this.state;

    let content;
    if (!executed) {
      content = <FrameLoading />;
    } else if (response) {
      content = <FrameSession frame={frame} response={response} data={data} />;
    } else if (successMessage) {
      content = (
        <FrameSuccess
          data={data}
          query={frame.query}
          successMessage={successMessage}
        />
      );
    } else if (errorMessage) {
      content = (
        <FrameError
          errorMessage={errorMessage}
          data={data}
          query={frame.query}
        />
      );
    }

    return (
      <FrameLayout
        frame={frame}
        onDiscardFrame={onDiscardFrame}
        onSelectQuery={onSelectQuery}
        collapseAllFrames={collapseAllFrames}
        responseFetched={Boolean(response)}
        onAfterExpandFrame={this.executeFrameQuery}
        onAfterCollapseFrame={this.cleanFrameData}
      >
        {content}
      </FrameLayout>
    );
  }
}

export default FrameItem;

import React from "react";

import FrameLayout from "./FrameLayout";
import FrameSession from "./FrameSession";
import FrameError from "./FrameError";
import FrameSuccess from "./FrameSuccess";
import FrameLoading from "./FrameLoading";

import { executeQuery, isNotEmpty } from "../lib/helpers";
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
    const { query, meta } = frame;

    if (!meta.collapsed) {
      this.executeFrameQuery(query);
    }
  }

  executeFrameQuery = query => {
    const { frame: { meta } } = this.props;

    executeQuery(query)
      .then(res => {
        if (res.errors) {
          // Handle query error responses here.
          this.setState({
            errorMessage: res.errors[0].message,
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
              data: res.data,
              executed: true
            });
          } else {
            this.setState({
              successMessage: res.data.message,
              data: res.data,
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
            data: res.data
          };

          this.setState({ response, executed: true });
        } else {
          this.setState({
            successMessage: "Your query did not return any results",
            executed: true,
            data: res.data
          });
        }
      })
      .catch(error => {
        console.log(error);
        this.setState({ error, executed: true });
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
      content = <FrameSession frame={frame} response={response} />;
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
      >
        {content}
      </FrameLayout>
    );
  }
}

export default FrameItem;

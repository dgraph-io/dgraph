import React from "react";

import FrameLayout from "./FrameLayout";
import FrameSession from "./FrameSession";
import FrameError from "./FrameError";
import FrameSuccess from "./FrameSuccess";
import FrameLoading from "./FrameLoading";

import { executeQuery } from "../lib/helpers";
import { processGraph } from "../lib/graph";

class FrameItem extends React.Component {
  constructor(props) {
    super(props);

    this.state = {
      response: null,
      error: null,
      executed: false
    };
  }

  componentDidMount() {
    const { frame } = this.props;
    const { query, meta } = frame;

    if (!meta.collapsed) {
      console.log("running");
      this.executeFrameQuery(query);
    }
  }

  executeFrameQuery = query => {
    const { frame: { meta } } = this.props;
    executeQuery(query)
      .then(res => {
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

        console.log(response);

        this.setState({ response, executed: true });
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
    const { response, error, executed } = this.state;

    let content;
    if (!executed) {
      content = <FrameLoading />;
    } else if (response) {
      content = <FrameSession frame={frame} response={response} />;
    } else if (error) {
      // content = <FrameError data={error} />;
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

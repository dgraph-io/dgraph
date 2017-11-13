import React from "react";
import pluralize from "pluralize";

import { humanizeTime, serverLatency } from "../lib/helpers";

const SessionFooterResult = ({
  graphRenderTime,
  treeRenderTime,
  currentTab,
  response
}) => {
  let currentAction;
  if (currentTab === "graph" || currentTab === "tree") {
    currentAction = "Showing";
  } else {
    currentAction = "Found";
  }

  return (
    <div className="row">
      <div className="col-12 col-sm-8">
        <i className="fa fa-check check-mark" />{" "}
        <span className="result-message">
          {currentAction} <span className="value">
            {response.numNodes}
          </span>{" "}
          {pluralize("node", response.numNodes)} and{" "}
          <span className="value">{response.numEdges}</span>{" "}
          {pluralize("edge", response.numEdges)}
        </span>
      </div>
      <div className="col-12 col-sm-4">
        <div className="latency stats">
          {response.data.extensions &&
          response.data.extensions.server_latency ? (
            <div className="stat">
              Server latency:{" "}
              <span className="value">
                {serverLatency(response.data.extensions.server_latency)}
              </span>
            </div>
          ) : null}
          {graphRenderTime && currentTab === "graph" ? (
            <div className="stat">
              Rendering latency:{" "}
              <span className="value">{humanizeTime(graphRenderTime)}</span>
            </div>
          ) : null}
          {treeRenderTime && currentTab === "tree" ? (
            <div className="stat">
              Rendering latency:{" "}
              <span className="value">{humanizeTime(treeRenderTime)}</span>
            </div>
          ) : null}
        </div>
      </div>
    </div>
  );
};

export default SessionFooterResult;

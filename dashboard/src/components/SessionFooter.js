import React from "react";

import SessionFooterResult from "./SessionFooterResult";
import SessionFooterProperties from "./SessionFooterProperties";
import SessionFooterConfig from "./SessionFooterConfig";

const SessionFooter = ({
  response,
  currentTab,
  graphRenderTime,
  treeRenderTime,
  hoveredNode,
  selectedNode,
  configuringNodeType,
  isConfiguringLabel,
  data
}) => {
  let child;
  if (isConfiguringLabel) {
    child = <SessionFooterConfig />;
  } else if (selectedNode || hoveredNode) {
    child = <SessionFooterProperties entity={selectedNode || hoveredNode} />;
  } else {
    child = (
      <SessionFooterResult
        currentTab={currentTab}
        response={response}
        graphRenderTime={graphRenderTime}
        treeRenderTime={treeRenderTime}
      />
    );
  }

  return <div className="footer">{child}</div>;
};
export default SessionFooter;

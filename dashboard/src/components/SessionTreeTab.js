import React from "react";

import GraphContainer from "../containers/GraphContainer";

const SessionTreeTab = ({
  session,
  active,
  onBeforeTreeRender,
  onTreeRendered,
  onNodeSelected,
  onNodeHovered,
  selectedNode,
  nodesDataset,
  edgesDataset
}) => {
  return (
    <div className="content-container">
      <GraphContainer
        response={session.response}
        onBeforeRender={onBeforeTreeRender}
        onRendered={onTreeRendered}
        onNodeSelected={onNodeSelected}
        onNodeHovered={onNodeHovered}
        selectedNode={selectedNode}
        nodesDataset={nodesDataset}
        edgesDataset={edgesDataset}
        treeView
      />
    </div>
  );
};

export default SessionTreeTab;

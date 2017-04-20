import React from 'react';
import CSSTransitionGroup from 'react-transition-group/CSSTransitionGroup';

import NodeOrEdgeInfo from '../components/NodeOrEdgeInfo';
import Label from '../components/Label';
import PartialGraphFooter from '../components/PartialGraphFooter';

const GraphFooter = ({
  response, selectedNode, hoveredNode, canToggleExpand, partiallyRendered,
  onExpandNetwork, onCollapseNetwork
}) => {
  const focusedNode = hoveredNode || selectedNode;

  return (
    <div className="graph-footer">
      <div className="labels">
        {response.plotAxis.map((label, i) => {
          return (
            <Label
              key={i}
              color={label.color}
              pred={label.pred}
              label={label.label}
            />
          );
        })}
      </div>

      <CSSTransitionGroup
        transitionName="properties"
        transitionEnterTimeout={180}
        transitionLeaveTimeout={180}
      >
        {focusedNode ?
          <NodeOrEdgeInfo
            node={focusedNode}
          />
        : null}
      </CSSTransitionGroup>

      {canToggleExpand ?
        <PartialGraphFooter
          partiallyRendered={partiallyRendered}
          onExpandNetwork={onExpandNetwork}
          onCollapseNetwork={onCollapseNetwork}
        /> : null}
    </div>
  );
};
export default GraphFooter;

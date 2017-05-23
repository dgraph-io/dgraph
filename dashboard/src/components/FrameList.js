import React from "react";
import FrameItem from "./FrameItem";
import CSSTransitionGroup from "react-transition-group/CSSTransitionGroup";

import "../assets/css/Frames.css";

const FrameList = ({
  frames,
  onDiscardFrame,
  onSelectQuery,
  collapseAllFrames
}) => {
  return (
    <CSSTransitionGroup
      transitionName="frame-item"
      transitionEnterTimeout={300}
      transitionLeaveTimeout={300}
      component="ul"
      className="frame-list"
    >
      {frames.map(frame => {
        return (
          <FrameItem
            key={frame.id}
            frame={frame}
            onDiscardFrame={onDiscardFrame}
            onSelectQuery={onSelectQuery}
            collapseAllFrames={collapseAllFrames}
          />
        );
      })}
    </CSSTransitionGroup>
  );
};

export default FrameList;

import React from "react";

const PartialRenderInfo = ({
  partiallyRendered,
  onExpandNetwork,
  onCollapseNetwork
}) => {
  return (
    <div className="partial-render-info">
      {partiallyRendered
        ? <div>
            Only a subset of graph was rendered. <a
              href="#expand"
              onClick={e => {
                e.preventDefault();
                onExpandNetwork();
              }}
            >
              Expand 500 nodes.
            </a>
          </div>
        : <div>
            <a href="#collapse" onClick={onCollapseNetwork}>
              Render subset only.
            </a>
          </div>}
    </div>
  );
};
export default PartialRenderInfo;

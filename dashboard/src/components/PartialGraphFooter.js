import React from 'react';

const PartialGraphFooter = ({ partiallyRendered, onExpandNetwork, onCollapseNetwork }) => {
  return (
    <div className="partial-graph-footer">
      {partiallyRendered ?
        <div>
          Only a subset of graph was rendered.
          <a
            href="#expand"
            className="btn btn-sm btn-primary"
            onClick={(e) => {
              e.preventDefault();
              onExpandNetwork();
            }}>
            Expand 500 nodes
          </a>
        </div>
        :
        <div>
          <a
            href="#collapse"
            className="btn btn-sm btn-primary"
            onClick={onCollapseNetwork}
          >Collapse nodes</a>
        </div>
      }
    </div>
  );
};
export default PartialGraphFooter;

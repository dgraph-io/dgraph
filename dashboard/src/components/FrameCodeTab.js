import React from 'react';

import Highlight from './Highlight';

const FrameCodeTab = ({ query, response }) => {
  return (
    <div className="content-container">
      <div className="code-container">
        <div className="code-header">
          <span className="label label-info">
            Query
          </span>
        </div>
        <Highlight preClass="content">
          {query}
        </Highlight>
      </div>

      <div className="code-container">
        <div className="code-header">
          <span className="label label-info">
            Response
          </span>
        </div>
        <Highlight preClass="content">
          {JSON.stringify(response, null, 2)}
        </Highlight>
      </div>
    </div>
  );
};
export default FrameCodeTab;

import React from 'react';

import Highlight from './Highlight';

const FrameQueryTab = ({ query, response }) => {
  return (
    <div className="content-container">
      <Highlight preClass="content">
        {query}
      </Highlight>
    </div>
  );
};
export default FrameQueryTab;

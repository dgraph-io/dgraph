import React from 'react';

import { collapseQuery } from '../lib/helpers';

const QueryPreview = ({ query, onSelectQuery }) => {
  return (
    <div
      className="query-row"
      onClick={(e) => {
        e.preventDefault();
        onSelectQuery(query);

        // Scroll to top
        // IDEA: This breaks encapsulation. Is there a better way?
        document.querySelector('.main-content').scrollTop = 0;
      }}
    >
      <i className="fa fa-search query-prompt" /> <span className="preview">{collapseQuery(query)}</span>
    </div>
  );
};

export default QueryPreview;

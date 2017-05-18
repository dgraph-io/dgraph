// @flow

import React, { Component } from "react";

// import "../assets/css/Properties.css";

class Properties extends Component {
  render() {
    const { entity } = this.props;
    const nodeProperties = JSON.parse(entity.title);

    // Nodes have facets and attrs keys.
    const isEdge = Object.keys(nodeProperties).length === 1;
    const attrs = nodeProperties.attrs || {};
    const facets = nodeProperties.facets || {};

    return (
      <div className="properties">
        <span>Showing {isEdge ? "edge" : "node"}:</span>
        {!isEdge &&
          <div>
            <ul className="Properties">
              {Object.keys(attrs).map(function(key, idx) {
                return (
                  <li className="Properties-key-val" key={idx}>
                    <span className="Properties-key">
                      {key}:
                    </span>
                    <span className="Properties-val">
                      {String(attrs[key])}
                    </span>
                  </li>
                );
              })}
            </ul>
          </div>}
        {Object.keys(facets).length > 0 &&
          !isEdge &&
          <span className="Properties-facets">Facets</span>}
        <ul className="Properties">
          {Object.keys(facets).map(function(key, idx) {
            return (
              <li className="Properties-key-val" key={idx}>
                <span className="Properties-key">
                  {key}:
                </span>
                <span className="Properties-val">
                  {String(facets[key])}
                </span>
              </li>
            );
          })}
        </ul>
      </div>
    );
  }
}

export default Properties;

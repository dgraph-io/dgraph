// @flow

import React, { Component } from "react";

import "../assets/css/Properties.css";

class Properties extends Component {
    render() {
        let props = JSON.parse(this.props.currentNode),
            // Nodes have facets and attrs keys.
            isEdge = Object.keys(props).length === 1,
            attrs = props["attrs"] || {},
            facets = props["facets"] || {};

        return (
            <div id="properties">
                {isEdge ? "Edge" : "Node"} Attributes:
                {!isEdge &&
                    <div className="Properties">
                        {Object.keys(attrs).map(function(key, idx) {
                            return (
                                <div className="Properties-key-val" key={idx}>
                                    <div className="Properties-key">
                                        {key}:&nbsp;
                                    </div>
                                    <div className="Properties-val">
                                        {String(attrs[key])}
                                    </div>
                                </div>
                            );
                        })}
                    </div>}
                {Object.keys(facets).length > 0 &&
                    !isEdge &&
                    <span className="Properties-facets">Facets</span>}
                <div className="Properties">
                    {Object.keys(facets).map(function(key, idx) {
                        return (
                            <div className="Properties-key-val" key={idx}>
                                <div className="Properties-key">
                                    {key}:&nbsp;
                                </div>
                                <div className="Properties-val">
                                    {String(facets[key])}
                                </div>
                            </div>
                        );
                    })}
                </div>
            </div>
        );
    }
}

export default Properties;

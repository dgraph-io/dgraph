// @flow

import React, { Component } from "react";
import { Button, Glyphicon } from "react-bootstrap";

import "../assets/css/Properties.css";

class Properties extends Component {
    render() {
        let props = this.props.currentNode !== "{}" &&
            JSON.parse(this.props.currentNode.title),
            name = this.props.currentNode.name,
            uid = this.props.currentNode.uid,
            // Nodes have facets and attrs keys.
            isEdge = Object.keys(props).length === 1,
            attrs = props["attrs"] || {},
            facets = props["facets"] || {};

        return (
            <div id="properties">
                {isEdge
                    ? <span>Edge Attributes:</span>
                    : <div>
                          <span>Node Attributes:</span>
                          <Button
                              title="Add uid to scratchpad"
                              style={{ marginLeft: "5px" }}
                              bsSize="xsmall"
                              onClick={() => {
                                  this.props.addEntry(uid, name);
                              }}
                          >
                              <Glyphicon glyph="save" />
                          </Button>
                      </div>}
                {!isEdge &&
                    <div>
                        <div className="Properties">
                            {Object.keys(attrs).map(function(key, idx) {
                                return (
                                    <div
                                        className="Properties-key-val"
                                        key={idx}
                                    >
                                        <div className="Properties-key">
                                            {key}:&nbsp;
                                        </div>
                                        <div className="Properties-val">
                                            {String(attrs[key])}
                                        </div>
                                    </div>
                                );
                            })}
                        </div>
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

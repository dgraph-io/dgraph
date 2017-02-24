// @flow

import React, { Component } from "react";

class Properties extends Component {
    render() {
        return (
            <div id="properties" style={{ marginTop: "5px" }}>
                Current Node:<div
                    className="App-properties"
                    title={this.props.currentNode}
                >
                    <em>
                        <pre style={{ fontSize: "10px" }}>
                            {JSON.stringify(
                                JSON.parse(this.props.currentNode),
                                null,
                                2,
                            )}
                        </pre>
                    </em>
                </div>
            </div>
        );
    }
}

export default Properties;

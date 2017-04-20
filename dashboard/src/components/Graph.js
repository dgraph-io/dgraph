import React from "react";

import "../assets/css/Graph.css";

class Graph extends React.Component {
  render() {
    const { text, success } = this.props;

    return (
        <div className="Graph-wrapper">
            <div className="Graph-outer">
                <div id="graph" />
            </div>
        </div>
    );
  }
}

export default Graph;

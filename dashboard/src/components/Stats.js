import React, { Component } from "react";

// TODO - Later have one css file per component.
import "../assets/css/App.css";

class Stats extends Component {
  render() {
    const display = (
      <span>
        {this.props.server !== "" &&
          <span>
            <span>Server Latency: </span><b>{this.props.server}</b>,{" "}
          </span>}
        {this.props.rendering !== "" &&
          <span><span>Rendering: </span><b>{this.props.rendering}</b></span>}
      </span>
    );
    return (
      <div style={{ marginTop: "5px" }} className="App-stats">
        <span>
          {this.props.server !== "" || this.props.rendering !== ""
            ? display
            : ""}
        </span>
      </div>
    );
  }
}

export default Stats;

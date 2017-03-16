import React, { Component } from "react";

// TODO - Later have one css file per component.
import "../assets/css/App.css";

class Stats extends Component {
  render() {
    const display = (
      <span>
        Server Latency:{" "}
        <b>{this.props.server}</b>
        , Rendering:{" "}
        <b>{this.props.rendering}</b>
      </span>
    );
    return (
      <div style={{ marginTop: "5px" }} className="App-stats">
        <span>
          {this.props.server !== "" && this.props.rendering !== ""
            ? display
            : ""}
        </span>
      </div>
    );
  }
}

export default Stats;

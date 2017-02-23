import React, { Component } from "react";

// TODO - Later have one css file per component.
import "../assets/css/App.css";

class Stats extends Component {
  render() {
    const display = (
      <span>
        Server Latency:{" "}
        <b>{this.props.latency}</b>
        , Rendering:{" "}
        <b>{this.props.rendering}</b>
      </span>
    );
    return (
      <div
        style={{ marginTop: "5px" }}
        className={`App-stats ${this.props.class}`}
      >
        <span>
          {this.props.latency !== "" && this.props.rendering !== ""
            ? display
            : ""}
        </span>
      </div>
    );
  }
}

export default Stats;

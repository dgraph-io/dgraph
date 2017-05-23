// @flow

import React, { Component } from "react";

function getTextColor(bgColor) {
  var nThreshold = 105;
  var components = getRGBComponents(bgColor);
  var bgDelta =
    components.R * 0.299 + components.G * 0.587 + components.B * 0.114;

  return 255 - bgDelta < nThreshold ? "#000000" : "#ffffff";
}

function getRGBComponents(color) {
  var r = color.substring(1, 3);
  var g = color.substring(3, 5);
  var b = color.substring(5, 7);

  return {
    R: parseInt(r, 16),
    G: parseInt(g, 16),
    B: parseInt(b, 16)
  };
}

class Label extends Component {
  render() {
    return (
      <div
        className="label-container"
        style={{
          backgroundColor: this.props.color,
          color: getTextColor(this.props.color)
        }}
      >
        <span className="label-value">{this.props.pred}</span>
        <span className="shorthand">({this.props.label})</span>
      </div>
    );

    // const { onInitNodeTypeConfig } = this.props;
    //
    // return (
    //   <a
    //     href="#init-config"
    //     className="label-container"
    //     style={{
    //       backgroundColor: this.props.color,
    //       color: getTextColor(this.props.color)
    //     }}
    //     onClick={e => {
    //       e.preventDefault();
    //
    //       onInitNodeTypeConfig(this.props.pred);
    //     }}
    //   >
    //     <span className="label-value">{this.props.pred}</span>
    //     <span className="shorthand">({this.props.label})</span>
    //   </a>
    // );
  }
}

export default Label;

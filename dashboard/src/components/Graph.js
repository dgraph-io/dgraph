import React from "react";

import Label from "../components/Label";

import "../assets/css/Graph.css";

function Graph(props) {
    let { text, success, plotAxis, fs } = props;
    let graphClass = fs ? "Graph-fs" : "Graph-s";
    return (
        <div className="Graph-wrapper">
            <div className={fs ? "Graph-full-height" : "Graph-fixed-height"}>
                <div id="graph" className={`Graph ${graphClass}`}>
                    {text}
                </div>
            </div>
            <div className="Graph-label-box">
                <div className="Graph-label">
                    {plotAxis.map(
                        function(label, i) {
                            return (
                                <Label
                                    key={i}
                                    color={label.color}
                                    pred={label.pred}
                                    label={label.label}
                                />
                            );
                        },
                        this,
                    )}
                </div>
            </div>
        </div>
    );
}

export default Graph;
// var graphClass = classNames( //     { Graph: true, "Graph-fs": this.props.graph === "fullscreen" }, //     { "Graph-s": this.props.graph !== "fullscreen" }, //     { "Graph-error": this.props.resType === "error-res" }, //     { "Graph-success": this.props.resType === "success-res" }, //     { "Graph-hourglass": this.props.resType === "hourglass" }, // );

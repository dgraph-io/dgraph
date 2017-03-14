import React from "react";

import Label from "../components/Label";
import ProgressBarContainer from "../containers/ProgressBarContainer";

import "../assets/css/Graph.css";

function Graph(props) {
    let { text, success, plotAxis, fs, isFetching } = props;
    let graphClass = fs ? "Graph-fs" : "Graph-s";
    let bgColor;
    if (success) {
        if (text !== "") {
            bgColor = "Graph-success";
        } else {
            bgColor = "";
        }
    } else {
        bgColor = "Graph-error";
    }
    let hourglass = isFetching ? "Graph-hourglass" : "";

    return (
        <div className="Graph-wrapper">
            <div className={fs ? "Graph-full-height" : "Graph-fixed-height"}>
                <ProgressBarContainer />
                <div
                    id="graph"
                    className={`Graph ${graphClass} ${bgColor} ${hourglass}`}
                >
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
                        this
                    )}
                </div>
            </div>
        </div>
    );
}

export default Graph;

import React from "react";
import { Nav, NavItem } from "react-bootstrap";

import Label from "../components/Label";
import ProgressBarContainer from "../containers/ProgressBarContainer";

import "../assets/css/Graph.css";

function Graph(props) {
    let {
        text,
        success,
        plotAxis,
        fs,
        isFetching,
        json,
        selectTab,
        selectedTab
    } = props,
        graphClass = fs ? "Graph-fs" : "Graph-s",
        bgColor,
        hourglass = isFetching ? "Graph-hourglass" : "",
        graphHeight = fs ? "Graph-full-height" : "Graph-fixed-height",
        showGraph = selectedTab === "1" ? "" : "Graph-hide",
        showJSON = selectedTab === "2" ? "" : "Graph-hide";

    if (success) {
        if (text !== "") {
            bgColor = "Graph-success";
        } else {
            bgColor = "";
        }
    } else if (text !== "") {
        bgColor = "Graph-error";
    }

    return (
        <div className="Graph-wrapper">
            <Nav bsStyle="tabs" activeKey={selectedTab} onSelect={selectTab}>
                <NavItem eventKey="1" href="" title="Graph">Graph</NavItem>
                <NavItem eventKey="2" title="JSON">JSON</NavItem>
            </Nav>
            <div className="Graph-outer">
                <div className={`${graphHeight} ${showGraph}`}>
                    <ProgressBarContainer />
                    <div
                        id="graph"
                        className={
                            `Graph ${graphClass} ${bgColor} ${hourglass}`
                        }
                    >
                        {text}
                    </div>
                </div>
                <div className={`Graph-label-box ${showGraph}`}>
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
            <div className={`${graphHeight} ${showJSON} Graph-json`}>
                <div>
                    <pre><code>{JSON.stringify(json, null, 2)}</code></pre>
                </div>
            </div>
        </div>
    );
}

export default Graph;

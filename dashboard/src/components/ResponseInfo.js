import React from "react";

import StatsContainer from "../containers/StatsContainer";
import PropertiesContainer from "../containers/PropertiesContainer";

import { Button } from "react-bootstrap";

import "../assets/css/ResponseInfo.css";

// TODO - Have a foolproof logic for expand/collapse based on initial number of
// nodes rendered, total nodes and current number of nodes in the Graph.
const ResponseInfo = (
    {
        query,
        result,
        partial,
        rendering,
        latency,
        numNodes,
        numEdges,
        numNodesRendered,
        numEdgesRendered,
        treeView,
        renderGraph,
        expand
    }
) => (
    <div className="ResponseInfo">
        <PropertiesContainer />
        <div className="ResponseInfo-stats">
            <div className="ResponseInfo-flex">
                <StatsContainer />
                <div>
                    Nodes:{" "}
                    {numNodes}
                    , Edges:{" "}
                    {numEdges}
                </div>
            </div>
            <div className="ResponseInfo-flex">
                <Button
                    className="ResponseInfo-button"
                    bsStyle="primary"
                    disabled={
                        numNodes === 0 ||
                            (numNodesRendered === numNodes &&
                                numEdgesRendered === numEdges)
                    }
                    onClick={() => expand()}
                >
                    {partial ? "Expand" : "Collapse"}
                </Button>
                <Button
                    className="ResponseInfo-button"
                    bsStyle="primary"
                    disabled={numNodes === 0}
                    onClick={() => renderGraph(query, result, !treeView)}
                >
                    {treeView ? "Graph view" : "Tree View"}

                </Button>
            </div>
        </div>
        <div className="ResponseInfo-partial">
            <i>
                {partial
                    ? "We have only loaded a subset of the graph. Double click on a leaf node to expand its child nodes."
                    : ""}
            </i>
        </div>
    </div>
);

export default ResponseInfo;

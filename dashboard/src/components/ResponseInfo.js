import React from "react";

import Stats from "../components/Stats";
import Properties from "../components/Properties";

import { Button } from "react-bootstrap";

import "../assets/css/ResponseInfo.css";

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
        treeView,
        currentNode,
        renderGraph,
        expand,
    },
) => (
    <div className="ResponseInfo">
        <Properties currentNode={currentNode} />
        <div className="ResponseInfo-stats">
            <div className="ResponseInfo-flex">
                <Stats rendering={rendering} latency={latency} />
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
                    disabled={numNodes === 0 || numNodesRendered === numNodes}
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

import React, { Component } from "react";

import Stats from "./Stats";
import Graph from "./Graph";
// import { outgoingEdges } from "./Helpers";

import { Button } from "react-bootstrap";

import "../assets/css/App.css";

// function isExpanded(nodeId, edgeSet) {
//     if (outgoingEdges(nodeId, edgeSet).length > 0) {
//         return true;
//     }

//     return outgoingEdges(nodeId, this.props.allEdgeSet).length === 0;
// }

// function childNodes(edges) {
//     return edges.map(function(edge) {
//         return edge.to;
//     });
// }

// function expandAll() {
//     if (network === undefined) {
//         return;
//     }

//     if (this.state.expandText === "Collapse") {
//         renderPartialGraph.bind(this, this.state.result)();
//         this.setState({
//             expandText: "Expand",
//         });
//         return;
//     }

//     let nodeIds = network.body.nodeIndices.slice(),
//         nodeSet = network.body.data.nodes,
//         edgeSet = network.body.data.edges,
//         // We add nodes and edges that have to be updated to these arrays.
//         nodesBatch = [],
//         edgesBatch = [],
//         batchSize = 1000,
//         nodes = [];
//     while (nodeIds.length > 0) {
//         let nodeId = nodeIds.pop();
//         // If is expanded, do nothing, else put child nodes and edges into array for
//         // expansion.
//         if (isExpanded(nodeId, edgeSet)) {
//             continue;
//         }

//         let outEdges = outgoingEdges(nodeId, this.props.allEdgeSet),
//             outNodeIds = childNodes(outEdges);

//         nodeIds = nodeIds.concat(outNodeIds);
//         nodes = this.props.allNodeSet.get(outNodeIds);
//         nodesBatch = nodesBatch.concat(nodes);
//         edgesBatch = edgesBatch.concat(outEdges);

//         if (nodesBatch.length > batchSize) {
//             nodeSet.update(nodesBatch);
//             edgeSet.update(edgesBatch);
//             nodesBatch = [];
//             edgesBatch = [];

//             if (nodeIds.length === 0) {
//                 this.setState({
//                     expandText: "Collapse",
//                     partial: false,
//                 });
//             }
//             return;
//         }
//     }

//     if (nodesBatch.length > 0 || edgesBatch.length > 0) {
//         this.setState({
//             expandText: "Collapse",
//             partial: false,
//         });
//         nodeSet.update(nodesBatch);
//         edgeSet.update(edgesBatch);
//     }
// }

class Response extends Component {
    render() {
        return (
            <div
                style={{ width: "100%", height: "100%", padding: "5px" }}
                id="response"
            >
                <Graph
                    allNodes={this.props.allNodes}
                    allEdges={this.props.allEdges}
                    plotAxis={this.props.plotAxis}
                />
                <div style={{ fontSize: "12px" }}>
                    <Stats
                        rendering={this.props.rendering}
                        latency={this.props.latency}
                        class="hidden-xs"
                    />
                    <Button
                        style={{ float: "right", marginRight: "10px" }}
                        bsStyle="primary"
                        disabled={this.props.expandDisabled}
                        // onClick={expandAll.bind(this)}
                    >
                        {this.props.expandText}
                    </Button>
                    <div>
                        Nodes: {this.props.nodes}, Edges: {this.props.relations}
                    </div>
                    <div style={{ height: "auto" }}>
                        <i>
                            {this.props.partial === true
                                ? "We have only loaded a subset of the graph. Double click on a leaf node to expand its child nodes."
                                : ""}
                        </i>
                    </div>
                    <div id="properties" style={{ marginTop: "5px" }}>
                        Current Node:<div
                            className="App-properties"
                            title={this.props.currentNode}
                        >
                            <em>
                                <pre style={{ fontSize: "10px" }}>
                                    {JSON.stringify(
                                        JSON.parse(this.props.currentNode),
                                        null,
                                        2,
                                    )}
                                </pre>
                            </em>
                        </div>
                    </div>
                </div>
            </div>
        );
    }
}

export default Response;

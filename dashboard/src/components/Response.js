import React, { Component } from "react";

import Stats from "./Stats";
import Graph from "./Graph";
import Properties from "./Properties";
import { outgoingEdges } from "./Helpers";

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
//     // if (network === undefined) {
//     //     return;
//     // }

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
    constructor(props: Props) {
        super(props);

        this.state = {
            currentNode: "{}",
        };
    }

    setCurrentNode = properties => {
        this.setState({
            currentNode: properties,
        });
    };

    render() {
        return (
            <div
                style={{ width: "100%", height: "100%", padding: "5px" }}
                id="response"
            >
                <Graph
                    allNodes={this.props.allNodes}
                    allEdges={this.props.allEdges}
                    nodes={this.props.nodes}
                    edges={this.props.edges}
                    response={this.props.response}
                    resType={this.props.resType}
                    graph={this.props.graph}
                    graphHeight={this.props.graphHeight}
                    plotAxis={this.props.plotAxis}
                    setCurrentNode={this.setCurrentNode}
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
                        disabled={this.props.nodes === this.props.allNodes}
                        // onClick={expandAll.bind(this)}
                    >
                        {this.props.expandText}
                    </Button>
                    <div>
                        Nodes:{" "}
                        {this.props.allNodes.length}
                        , Edges:{" "}
                        {this.props.allEdges.length}
                    </div>
                    <div style={{ height: "auto" }}>
                        <i>
                            {this.props.partial === true
                                ? "We have only loaded a subset of the graph. Double click on a leaf node to expand its child nodes."
                                : ""}
                        </i>
                    </div>
                    <Properties currentNode={this.state.currentNode} />
                </div>
            </div>
        );
    }
}

export default Response;

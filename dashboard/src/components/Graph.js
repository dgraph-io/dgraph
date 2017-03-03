import React, { Component } from "react";
import classNames from "classnames";
import vis from "vis";

import Label from "./Label";
import { outgoingEdges } from "./Helpers";

import "../assets/css/Graph.css";

function childNodes(edges) {
    return edges.map(function(edge) {
        return edge.to;
    });
}

var doubleClickTime = 0;
var threshold = 200;

function doOnClick(params, allNodeSet, edgeSet) {
    if (params.nodes.length > 0) {
        var nodeUid = params.nodes[0], currentNode = allNodeSet.get(nodeUid);

        this.setState({
            selectedNode: true,
        });
        this.props.setCurrentNode(currentNode.title);
    } else if (params.edges.length > 0) {
        var edgeUid = params.edges[0], currentEdge = edgeSet.get(edgeUid);
        this.setState({
            selectedNode: true,
        });
        this.props.setCurrentNode(currentEdge.title);
    } else {
        this.setState({
            selectedNode: false,
        });
        this.props.setCurrentNode("{}");
    }
}

// create a network
function renderNetwork(props) {
    var container = document.getElementById("graph");
    var data = {
        nodes: new vis.DataSet(props.nodes),
        edges: new vis.DataSet(props.edges),
    };
    var options = {
        nodes: {
            shape: "circle",
            scaling: {
                max: 20,
                min: 20,
                label: {
                    enabled: true,
                    min: 14,
                    max: 14,
                },
            },
            font: {
                size: 16,
            },
            margin: {
                top: 25,
            },
        },
        height: "100%",
        width: "100%",
        interaction: {
            hover: true,
            keyboard: {
                enabled: true,
                bindToWindow: false,
            },
            tooltipDelay: 1000000,
            hideEdgesOnDrag: true,
        },
        layout: {
            randomSeed: 42,
            improvedLayout: false,
        },
        physics: {
            stabilization: {
                fit: true,
            },
            // timestep: 0.4,
            barnesHut: {
                // gravitationalConstant: -80000,
                // springConstant: 0.1,
                // springLength: 10,
                // avoidOverlap: 0.8,
                // springConstant: 0.1,
                damping: 0.7,
            },
        },
    };

    if (props.treeView) {
        Object.assign(options, {
            layout: {
                hierarchical: {
                    sortMethod: "directed",
                },
            },
            physics: {
                // Otherwise there is jittery movement (existing nodes move
                // horizontally which doesn't look good) when you expand some nodes.
                enabled: false,
                barnesHut: {},
            },
        });
    }

    let network = new vis.Network(container, data, options);
    let allNodeSet = new vis.DataSet(props.allNodes);
    let allEdgeSet = new vis.DataSet(props.allEdges), that = this;

    function multiLevelExpand(nodeId) {
        let nodes = [nodeId], nodeStack = [nodeId], adjEdges = [];
        while (nodeStack.length !== 0) {
            let nodeId = nodeStack.pop();

            let outgoing = outgoingEdges(nodeId, allEdgeSet),
                adjNodeIds = outgoing.map(function(edge) {
                    return edge.to;
                });

            nodeStack = nodeStack.concat(adjNodeIds);
            nodes = nodes.concat(adjNodeIds);
            adjEdges = adjEdges.concat(outgoing);
            if (adjNodeIds.length > 3) {
                break;
            }
        }
        data.nodes.update(allNodeSet.get(nodes));
        data.edges.update(adjEdges);
    }

    network.on("doubleClick", function(params) {
        doubleClickTime = new Date();
        if (params.nodes && params.nodes.length > 0) {
            let clickedNodeUid = params.nodes[0],
                clickedNode = data.nodes.get(clickedNodeUid);

            network.unselectAll();
            that.props.setCurrentNode(clickedNode.title);
            that.setState({
                selectedNode: false,
            });

            let outgoing = outgoingEdges(clickedNodeUid, data.edges),
                expanded = outgoing.length > 0,
                allOutgoingEdges = outgoingEdges(clickedNodeUid, allEdgeSet);

            let adjacentNodeIds: Array<string> = allOutgoingEdges.map(function(
                edge,
            ) {
                return edge.to;
            });

            let adjacentNodes = allNodeSet.get(adjacentNodeIds);

            // TODO -See if we can set a meta property to a node to know that its
            // expanded or closed and avoid this computation.
            if (expanded) {
                // Collapse all child nodes recursively.
                let allEdges = outgoing.map(function(edge) {
                    return edge.id;
                });

                let allNodes = adjacentNodes.slice();

                while (adjacentNodeIds.length > 0) {
                    let node = adjacentNodeIds.pop();
                    let connectedEdges = outgoingEdges(node, data.edges);

                    let connectedNodes = connectedEdges.map(function(edge) {
                        return edge.to;
                    });

                    allNodes = allNodes.concat(connectedNodes);
                    allEdges = allEdges.concat(connectedEdges);
                    adjacentNodeIds = adjacentNodeIds.concat(connectedNodes);
                }

                data.nodes.remove(allNodes);
                data.edges.remove(allEdges);
                that.props.updateExpanded(false);
            } else {
                multiLevelExpand(clickedNodeUid);
            }
        }
    });

    network.on("click", function(params) {
        var t0 = new Date();
        if (t0 - doubleClickTime > threshold) {
            setTimeout(
                function() {
                    if (t0 - doubleClickTime > threshold) {
                        doOnClick.bind(that)(params, data.nodes, data.edges);
                    }
                },
                threshold,
            );
        }
    });

    window.onresize = function() {
        network !== undefined && network.fit();
    };

    network.on("hoverNode", function(params) {
        // Only change properties if no node is selected.
        if (that.state.selectedNode) {
            return;
        }
        if (params.node.length > 0) {
            let nodeUid: string = params.node,
                currentNode = data.nodes.get(nodeUid);

            that.props.setCurrentNode(currentNode.title);
        }
    });

    network.on("hoverEdge", function(params) {
        // Only change properties if no node is selected.
        if (that.state.selectedNode) {
            return;
        }
        if (params.edge.length > 0) {
            let edgeUid = params.edge, currentEdge = data.edges.get(edgeUid);
            that.props.setCurrentNode(currentEdge.title);
        }
    });

    network.on("dragEnd", function(params) {
        for (let i = 0; i < params.nodes.length; i++) {
            let nodeId: string = params.nodes[i];
            data.nodes.update({ id: nodeId, fixed: { x: true, y: true } });
        }
    });

    network.on("dragStart", function(params) {
        for (let i = 0; i < params.nodes.length; i++) {
            let nodeId: string = params.nodes[i];
            data.nodes.update({ id: nodeId, fixed: { x: false, y: false } });
        }
    });

    function isExpanded(nodeId, edgeSet) {
        return outgoingEdges(nodeId, edgeSet).length ===
            outgoingEdges(nodeId, allEdgeSet).length;
    }

    var expand = function() {
        if (network === undefined) {
            return;
        }

        if (this.props.fullyExpanded) {
            data.nodes.remove(data.nodes.getIds());
            data.edges.remove(data.edges.getIds());
            data.nodes.update(this.props.nodes);
            data.edges.update(this.props.edges);
            this.props.updateExpanded(false);
            network.fit();
            return;
        }

        let nodeIds = data.nodes.getIds(),
            nodeSet = data.nodes,
            edgeSet = data.edges,
            // We add nodes and edges that have to be updated to these arrays.
            nodesBatch = new Set(),
            edgesBatch = [],
            batchSize = 500;

        while (nodeIds.length > 0) {
            let nodeId = nodeIds.pop();
            // If is expanded, do nothing, else put child nodes and edges into array for
            // expansion.
            if (isExpanded.bind(this)(nodeId, edgeSet)) {
                continue;
            }

            let outEdges = outgoingEdges(nodeId, allEdgeSet),
                outNodeIds = childNodes(outEdges);

            nodeIds = nodeIds.concat(outNodeIds);

            for (let id of outNodeIds) {
                nodesBatch.add(id);
            }

            edgesBatch = edgesBatch.concat(outEdges);

            if (nodesBatch.size > batchSize) {
                nodeSet.update(allNodeSet.get(Array.from(nodesBatch)));
                edgeSet.update(edgesBatch);
                nodesBatch = new Set();
                edgesBatch = [];
                return;
            }
        }

        if (nodeIds.length === 0) {
            that.props.updateExpanded(true);
        }

        if (nodesBatch.size > 0 || edgesBatch.length > 0) {
            nodeSet.update(allNodeSet.get(Array.from(nodesBatch)));
            edgeSet.update(edgesBatch);
        }

        network.fit();
    };

    var fit = function() {
        network.fit();
    };

    this.setState({ expand: expand, fit: fit });
}

class Graph extends Component {
    constructor(props: Props) {
        super(props);

        this.state = {
            selectedNode: false,
            expand: function() {},
            fit: function() {},
        };
    }

    expandAll = () => {
        this.state.expand.bind(this)();
    };

    render() {
        var graphClass = classNames(
            { Graph: true, "Graph-fs": this.props.graph === "fullscreen" },
            { "Graph-s": this.props.graph !== "fullscreen" },
            { "Graph-error": this.props.resType === "error-res" },
            { "Graph-success": this.props.resType === "success-res" },
            { "Graph-hourglass": this.props.resType === "hourglass" },
        );
        return (
            <div
                style={{
                    display: "flex",
                    flexDirection: "column",
                    flex: "auto",
                }}
            >
                <div className={this.props.graphHeight}>
                    <div id="graph" className={graphClass}>
                        {this.props.response}
                    </div>
                </div>
                <div className="Graph-label-box">
                    <div className="Graph-label">
                        {this.props.plotAxis.map(
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

    componentWillReceiveProps = nextProps => {
        if (nextProps.graphHeight !== this.props.graphHeight) {
            this.state.fit();
        }
        if (
            // TODO - Check how to do a shallow check?
            nextProps.nodes.length === this.props.nodes.length &&
            nextProps.edges.length === this.props.edges.length &&
            nextProps.allNodes.length === this.props.allNodes.length &&
            nextProps.allEdges.length === this.props.allEdges.length &&
            nextProps.response === this.props.response &&
            nextProps.treeView === this.props.treeView
        ) {
            return;
        }

        renderNetwork.bind(this, nextProps)();
    };
}

export default Graph;

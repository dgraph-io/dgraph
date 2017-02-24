import React, { Component } from "react";
import classNames from "classnames";
import vis from "vis";

import Label from "./Label";
import { outgoingEdges } from "./Helpers";

// import "../assets/css/Graph.css";
import "../assets/css/App.css";

var doubleClickTime = 0;
var threshold = 200;

function doOnClick(params, allNodeSet) {
    if (params.nodes.length > 0) {
        var nodeUid = params.nodes[0], currentNode = allNodeSet.get(nodeUid);

        this.setState({
            selectedNode: true,
        });
        this.props.setCurrentNode(currentNode.title);
    } else {
        this.setState({
            selectedNode: false,
        });
        this.props.setCurrentNode("{}");
    }
}

// create a network
function renderNetwork(nodes: Array<Node>, edges: Array<Edge>) {
    var container = document.getElementById("graph");
    var data = {
        nodes: new vis.DataSet(nodes),
        edges: new vis.DataSet(edges),
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
            improvedLayout: false,
        },
        physics: {
            stabilization: {
                fit: true,
                // updateInterval: 1000
            },
            // timestep: 0.2,
            barnesHut: {
                // gravitationalConstant: -80000,
                // springConstant: 0.1,
                // springLength: 10,
                // avoidOverlap: 0.8,
                // springConstant: 0.1,
                damping: 0.6,
            },
        },
    };

    let network = new vis.Network(container, data, options);
    let that = this;
    let allNodeSet = new vis.DataSet(this.props.allNodes);
    let allEdgeSet = new vis.DataSet(this.props.allEdges);

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
                // that.setState({
                //     expandText: "Expand",
                //     expandDisabled: false,
                // });
            } else {
                data.nodes.update(adjacentNodes);
                data.edges.update(allOutgoingEdges);
            }
        }
    });

    network.on("click", function(params) {
        var t0 = new Date();
        if (t0 - doubleClickTime > threshold) {
            setTimeout(
                function() {
                    if (t0 - doubleClickTime > threshold) {
                        doOnClick.bind(that)(params, allNodeSet);
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
        if (params.node === undefined) {
            return;
        }
        let nodeUid: string = params.node,
            currentNode = allNodeSet.get(nodeUid);

        that.props.setCurrentNode(currentNode.title);
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
}

class Graph extends Component {
    constructor(props: Props) {
        super(props);

        this.state = {
            network: {},
            selectedNode: false,
        };
    }

    render() {
        var graphClass = classNames(
            { "graph-s": true, fullscreen: this.props.graph === "fullscreen" },
            { "App-graph": this.props.graph !== "fullscreen" },
            { "error-res": this.props.resType === "error-res" },
            { "success-res": this.props.resType === "success-res" },
            { hourglass: this.props.resType === "hourglass" },
        );
        return (
            <div>
                <div className={this.props.graphHeight}>
                    <div id="graph" className={graphClass}>
                        {this.props.response}
                    </div>
                </div>
                <div
                    style={{
                        padding: "5px",
                        borderWidth: "0px 1px 1px 1px ",
                        borderStyle: "solid",
                        borderColor: "gray",
                        textAlign: "right",
                        margin: "0px",
                    }}
                >
                    <div style={{ marginRight: "10px", marginLeft: "auto" }}>
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
        if (
            // TODO - Check how to do a shallow check?
            nextProps.nodes.length === this.props.nodes.length &&
            nextProps.edges.length === this.props.edges.length &&
            nextProps.response === this.props.response
        ) {
            return;
        }
        this.setState({
            network: {},
        });

        renderNetwork.bind(this, nextProps.nodes, nextProps.edges)();
    };
}

export default Graph;

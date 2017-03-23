import React, { Component } from "react";
import { connect } from "react-redux";
import vis from "vis";

import Graph from "../components/Graph";
import {
    setCurrentNode,
    updatePartial,
    updateProgress,
    hideProgressBar,
    updateLatency
} from "../actions";
import { outgoingEdges } from "./Helpers";
import _ from "lodash/object";

import "../assets/css/Graph.css";

import "vis/dist/vis.min.css";

function childNodes(edges) {
    return edges.map(function(edge) {
        return edge.to;
    });
}

var doubleClickTime = 0;
var threshold = 200;

function doOnClick(params, allNodeSet, edgeSet, dispatch) {
    if (params.nodes.length > 0) {
        var nodeUid = params.nodes[0], currentNode = allNodeSet.get(nodeUid);

        this.setState({
            selectedNode: true
        });

        dispatch(setCurrentNode(currentNode));
    } else if (params.edges.length > 0) {
        var edgeUid = params.edges[0], currentEdge = edgeSet.get(edgeUid);
        this.setState({
            selectedNode: true
        });
        dispatch(setCurrentNode(currentEdge));
    } else {
        this.setState({
            selectedNode: false
        });
        dispatch(setCurrentNode("{}"));
    }
}

// create a network
function renderNetwork(props, dispatch) {
    var container = document.getElementById("graph");
    var data = {
        nodes: new vis.DataSet(props.nodes),
        edges: new vis.DataSet(props.edges)
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
                    max: 14
                }
            },
            font: {
                size: 16
            },
            margin: {
                top: 25
            }
        },
        height: "100%",
        width: "100%",
        interaction: {
            hover: true,
            keyboard: {
                enabled: true,
                bindToWindow: false
            },
            navigationButtons: true,
            tooltipDelay: 1000000,
            hideEdgesOnDrag: true
        },
        layout: {
            randomSeed: 42,
            improvedLayout: false
        },
        physics: {
            stabilization: {
                fit: true,
                updateInterval: 5,
                iterations: 20
            },
            // timestep: 0.4,
            barnesHut: {
                // gravitationalConstant: -80000,
                // springConstant: 0.1,
                // springLength: 10,
                // avoidOverlap: 0.8,
                // springConstant: 0.1,
                damping: 0.7
            }
        }
    };

    if (data.nodes.length < 100) {
        _.merge(options, {
            physics: {
                stabilization: {
                    iterations: 200,
                    updateInterval: 50
                }
            }
        });
    }

    if (props.treeView) {
        Object.assign(options, {
            layout: {
                hierarchical: {
                    sortMethod: "directed"
                }
            },
            physics: {
                // Otherwise there is jittery movement (existing nodes move
                // horizontally which doesn't look good) when you expand some nodes.
                enabled: false,
                barnesHut: {}
            }
        });
    }

    let network = new vis.Network(container, data, options),
        allNodeSet = new vis.DataSet(props.allNodes),
        allEdgeSet = new vis.DataSet(props.allEdges),
        that = this;

    this.setState({
        network: network
    });

    if (props.treeView) {
        // In tree view, physics is disabled and stabilizationIterationDone is not fired.
        dispatch(
            updateLatency({
                rendering: {
                    end: new Date()
                }
            })
        );
    }

    if (
        allNodeSet.length !== data.nodes.length ||
        allEdgeSet.length !== data.edges.length
    ) {
        dispatch(updatePartial(true));
    }

    network.on("stabilizationProgress", function(params) {
        var widthFactor = params.iterations / params.total;
        dispatch(updateProgress(widthFactor * 100));
    });

    network.once("stabilizationIterationsDone", function() {
        dispatch(hideProgressBar());
        dispatch(
            updateLatency({
                rendering: {
                    end: new Date()
                }
            })
        );
    });

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
            dispatch(setCurrentNode(clickedNode));
            that.setState({
                selectedNode: false
            });

            let outgoing = outgoingEdges(clickedNodeUid, data.edges),
                allOutgoingEdges = outgoingEdges(clickedNodeUid, allEdgeSet),
                expanded = outgoing.length > 0 || allOutgoingEdges.length === 0;

            let adjacentNodeIds: Array<string> = allOutgoingEdges.map(
                function(edge) {
                    return edge.to;
                }
            );

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
            } else {
                multiLevelExpand(clickedNodeUid);
                if (data.nodes.length === allNodeSet.length) {
                    dispatch(updatePartial(false));
                }
            }
        }
    });

    network.on("click", function(params) {
        var t0 = new Date();
        if (t0 - doubleClickTime > threshold) {
            setTimeout(
                function() {
                    if (t0 - doubleClickTime > threshold) {
                        doOnClick.bind(that)(
                            params,
                            data.nodes,
                            data.edges,
                            dispatch
                        );
                    }
                },
                threshold
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

            dispatch(setCurrentNode(currentNode));
        }
    });

    network.on("hoverEdge", function(params) {
        // Only change properties if no node is selected.
        if (that.state.selectedNode) {
            return;
        }
        if (params.edge.length > 0) {
            let edgeUid = params.edge, currentEdge = data.edges.get(edgeUid);
            dispatch(setCurrentNode(currentEdge));
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

        // This would be triggered when Collapse is pressed.
        if (!this.props.partial) {
            data.nodes.remove(data.nodes.getIds());
            data.edges.remove(data.edges.getIds());
            // Since we don't mutate the nodes and edges passed as props initially,
            // this still holds the initial state that was rendered and we can collapse
            // back the graph to that state.
            data.nodes.update(this.props.nodes);
            data.edges.update(this.props.edges);
            dispatch(updatePartial(true));
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
            dispatch(updatePartial(false));
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

class GraphContainer extends Component {
    constructor(props: Props) {
        super(props);

        this.state = {
            selectedNode: false,
            network: undefined,
            expand: function() {},
            fit: function() {}
        };
    }

    expand = () => {
        this.state.expand.bind(this)();
    };

    render() {
        const { plotAxis, text, success, fs, isFetching } = this.props;
        return (
            <Graph
                plotAxis={plotAxis}
                text={text}
                success={success}
                fs={fs}
                isFetching={isFetching}
            />
        );
    }

    componentWillReceiveProps = nextProps => {
        let network = this.state.network;

        if (nextProps.nodes.length === 0) {
            network && network.destroy();
            this.setState({ network: undefined });
            return;
        }

        if (nextProps.graphHeight !== this.props.graphHeight) {
            this.state.fit();
        }
        if (
            nextProps.nodes.length === this.props.nodes.length &&
            nextProps.edges.length === this.props.edges.length &&
            nextProps.allNodes.length === this.props.allNodes.length &&
            nextProps.allEdges.length === this.props.allEdges.length &&
            (nextProps.fs !== this.props.fs ||
                nextProps.partial !== this.props.partial)
        ) {
            return;
        }

        if (network !== undefined) {
            network.destroy();
            this.setState({ network: undefined });
        }
        renderNetwork.bind(this, nextProps, this.props.dispatch)();
    };
}

const mapStateToProps = state => ({
    ...state.response,
    partial: state.interaction.partial,
    fs: state.interaction.fullscreen
});

export default connect(mapStateToProps, null, null, { withRef: true })(
    GraphContainer
);

import React, { Component } from "react";

import Stats from "./Stats";
import Graph from "./Graph";
import Properties from "./Properties";

import { Button } from "react-bootstrap";

import "../assets/css/Response.css";

class Response extends Component {
    constructor(props: Props) {
        super(props);

        this.state = {
            currentNode: "{}",
            fullyExpanded: this.isFullyExpanded(this.props),
        };
    }

    componentWillReceiveProps = nextProps => {
        if (
            // TODO - Check how to do a shallow check?
            nextProps.nodes.length !== this.props.nodes.length ||
            nextProps.edges.length !== this.props.edges.length ||
            nextProps.allNodes.length !== this.props.allNodes.length ||
            nextProps.allEdges.length !== this.props.allEdges.length ||
            nextProps.treeView !== this.props.treeView
        ) {
            if (this.isFullyExpanded(nextProps)) {
                this.setState({
                    currentNode: "{}",
                    fullyExpanded: true,
                });
            } else {
                this.setState({
                    currentNode: "{}",
                    fullyExpanded: false,
                });
            }
        }
    };

    setCurrentNode = properties => {
        this.setState({
            currentNode: properties,
        });
    };

    updateExpanded = expanded => {
        this.setState({
            fullyExpanded: expanded,
        });
    };

    isFullyExpanded = props => {
        return props.allNodes.length > 0 &&
            props.allNodes.length === props.nodes.length &&
            props.allEdges.length === props.edges.length;
    };

    render() {
        return (
            <div className="Response" id="response">
                <Graph
                    ref="graph"
                    allNodes={this.props.allNodes}
                    allEdges={this.props.allEdges}
                    nodes={this.props.nodes}
                    edges={this.props.edges}
                    response={this.props.response}
                    result={this.props.result}
                    resType={this.props.resType}
                    graph={this.props.graph}
                    graphHeight={this.props.graphHeight}
                    plotAxis={this.props.plotAxis}
                    setCurrentNode={this.setCurrentNode}
                    updateExpanded={this.updateExpanded}
                    fullyExpanded={this.state.fullyExpanded}
                    treeView={this.props.treeView}
                />
                <div style={{ fontSize: "13px", flex: "0 auto" }}>
                    <Properties currentNode={this.state.currentNode} />
                    <div style={{ display: "flex" }}>
                        <div style={{ flex: "0 0 60%" }}>
                            <Stats
                                rendering={this.props.rendering}
                                latency={this.props.latency}
                            />
                            <div>
                                Nodes:{" "}
                                {this.props.allNodes.length}
                                , Edges:{" "}
                                {this.props.allEdges.length}
                            </div>
                        </div>
                        <div style={{ flex: "0 0 40%" }}>
                            <Button
                                className="Response-button"
                                bsStyle="primary"
                                disabled={
                                    this.props.allNodes.length === 0 ||
                                        this.isFullyExpanded(this.props)
                                }
                                onClick={() => this.refs.graph.expandAll()}
                            >
                                {this.state.fullyExpanded
                                    ? "Collapse"
                                    : "Expand"}
                            </Button>
                            <Button
                                className="Response-button"
                                bsStyle="primary"
                                disabled={this.props.allNodes.length === 0}
                                onClick={() =>
                                    this.props.renderGraph(
                                        this.props.result,
                                        !this.props.treeView,
                                    )}
                            >
                                {this.props.treeView
                                    ? "Graph view"
                                    : "Tree View"}
                            </Button>
                        </div>
                    </div>
                    <div style={{ height: "auto" }}>
                        <i>
                            {this.props.allNodes.length !== 0 &&
                                !this.state.fullyExpanded
                                ? "We have only loaded a subset of the graph. Double click on a leaf node to expand its child nodes."
                                : ""}
                        </i>
                    </div>
                </div>
            </div>
        );
    }
}

export default Response;

import React, { Component } from "react";

import Stats from "./Stats";
import Graph from "./Graph";
import Properties from "./Properties";
// import { outgoingEdges } from "./Helpers";

import { Button } from "react-bootstrap";

import "../assets/css/App.css";

class Response extends Component {
    constructor(props: Props) {
        super(props);

        this.state = {
            currentNode: "{}",
            fullyExpanded: this.isFullyExpanded(),
        };
    }

    componentWillReceiveProps = nextProps => {
        if (
            // TODO - Check how to do a shallow check?
            nextProps.nodes.length !== this.props.nodes.length ||
            nextProps.edges.length !== this.props.edges.length
        ) {
            this.setState({
                currentNode: "{}",
                fullyExpanded: false,
            });
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

    isFullyExpanded = () => {
        return this.props.allNodes.length > 0 &&
            this.props.allNodes.length === this.props.nodes.length &&
            this.props.allEdges.length === this.props.edges.length;
    };

    render() {
        return (
            <div
                style={{ width: "100%", height: "100%", padding: "5px" }}
                id="response"
            >
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
                        disabled={
                            this.props.allNodes.length === 0 ||
                                this.isFullyExpanded()
                        }
                        onClick={() => this.refs.graph.expandAll()}
                    >
                        {this.state.fullyExpanded ? "Collapse" : "Expand"}
                    </Button>
                    <div>
                        Nodes:{" "}
                        {this.props.allNodes.length}
                        , Edges:{" "}
                        {this.props.allEdges.length}
                    </div>
                    <div style={{ height: "auto" }}>
                        <i>
                            {this.props.allNodes.length !== 0 &&
                                !this.state.fullyExpanded
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

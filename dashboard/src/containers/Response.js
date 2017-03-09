import React, { Component } from "react";

import GraphContainer from "./GraphContainer";
import ResponseInfoContainer from "./ResponseInfoContainer";

import "../assets/css/Response.css";

class Response extends Component {
    constructor(props: Props) {
        super(props);
        this.expand = this.expand.bind(this);
    }

    expand = () => {
        // TODO - Maybe check how can we get rid of this ref stuff.
        this.refs.graph.getWrappedInstance().expand();
    };

    render() {
        return (
            <div className="Response" id="response">
                <GraphContainer ref="graph" />
                <ResponseInfoContainer expand={this.expand} />
            </div>
        );
    }
}

export default Response;

// ref="graph"
// allNodes={this.props.allNodes}
// allEdges={this.props.allEdges}
// nodes={this.props.nodes}
// edges={this.props.edges}
// response={this.props.response}
// result={this.props.result}
// resType={this.props.resType}
// graph={this.props.graph}
// graphHeight={this.props.graphHeight}
// plotAxis={this.props.plotAxis}
// setCurrentNode={this.setCurrentNode}
// updateExpanded={this.updateExpanded}
// fullyExpanded={this.state.fullyExpanded}
// treeView={this.props.treeView}

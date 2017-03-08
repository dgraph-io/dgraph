import React, { Component } from "react";

import GraphContainer from "./GraphContainer";
import ResponseInfoContainer from "./ResponseInfoContainer";

import "../assets/css/Response.css";

class Response extends Component {
    // constructor(props: Props) {
    //     super(props);

    //     this.state = {
    //         currentNode: "{}",
    //         fullyExpanded: this.isFullyExpanded(this.props),
    //     };
    // }

    // componentWillReceiveProps = nextProps => {
    //     if (
    //         // TODO - Check how to do a shallow check?
    //         nextProps.nodes.length !== this.props.nodes.length ||
    //         nextProps.edges.length !== this.props.edges.length ||
    //         nextProps.allNodes.length !== this.props.allNodes.length ||
    //         nextProps.allEdges.length !== this.props.allEdges.length ||
    //         nextProps.treeView !== this.props.treeView
    //     ) {
    //         if (this.isFullyExpanded(nextProps)) {
    //             this.setState({
    //                 currentNode: "{}",
    //                 fullyExpanded: true,
    //             });
    //         } else {
    //             this.setState({
    //                 currentNode: "{}",
    //                 fullyExpanded: false,
    //             });
    //         }
    //     }
    // };

    // setCurrentNode = properties => {
    //     this.setState({
    //         currentNode: properties,
    //     });
    // };

    // updateExpanded = expanded => {
    //     this.setState({
    //         fullyExpanded: expanded,
    //     });
    // };

    isFullyExpanded = props => {
        return false;
        // return props.allNodes.length > 0 &&
        //     props.allNodes.length === props.nodes.length &&
        //     props.allEdges.length === props.edges.length;
    };

    render() {
        return (
            // TODO - Check why does this have id?
            (
                <div className="Response" id="response">
                    <GraphContainer />
                    <ResponseInfoContainer />
                </div>
            )
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

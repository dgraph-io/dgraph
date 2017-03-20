import { connect } from "react-redux";

import ResponseInfo from "../components/ResponseInfo";
import { renderGraph } from "../actions";

// TODO - Ideally have separate container components for
// Properties, Buttons as they change within a query.

const mapStateToProps = (state, ownProps) => ({
    query: state.query.text,
    result: state.response.data,
    partial: state.interaction.partial,
    numNodesRendered: state.response.nodes.length,
    numEdgesRendered: state.response.edges.length,
    numNodes: state.response.numNodes,
    numEdges: state.response.numEdges,
    treeView: state.response.treeView,
    currentNode: state.interaction.node,
    expand: ownProps.expand
});

const mapDispatchToProps = dispatch => ({
    renderGraph: (query, result, treeView) => {
        dispatch(renderGraph(query, result, treeView));
    }
});

export default connect(mapStateToProps, mapDispatchToProps)(ResponseInfo);

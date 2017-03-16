import { connect } from "react-redux";

import Progress from "../components/Progress";

const mapStateToProps = state => ({
    display: state.interaction.progress.display,
    perc: state.interaction.progress.perc
});

export default connect(mapStateToProps, null)(Progress);

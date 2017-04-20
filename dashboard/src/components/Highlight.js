import React from 'react';
import classnames from 'classnames';
import hljs from 'highlight.js/lib/highlight';

export default class Highlight extends React.Component {
  constructor(props) {
    super(props);

    this.highlightCode = this.highlightCode.bind(this);
  }

  componentDidMount() {
    this.highlightCode();
  }

  componentDidUpdate() {
    this.highlightCode();
  }

  highlightCode() {
    const domNode = this.refs.code;
    hljs.registerLanguage('json', require('highlight.js/lib/languages/json'));

    hljs.highlightBlock(domNode);
  }

  render() {
    const { children, codeClass, preClass } = this.props;

    return (
      <pre className={classnames(preClass)}>
        <code className={classnames('json', codeClass)} ref="code">{children}</code></pre>
    );
  }
}

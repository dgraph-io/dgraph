import React from "react";

const SidebarFeedback = () => {
  return (
    <div className="sidebar-help">
      <h2>Feedback</h2>

      <p>
        How can <a
          href="https://dgraph.io/about.html"
          target="_blank"
          className="team-link"
        >
          we
        </a> improve Dgraph browser?
      </p>

      <section>
        <h3>Ways to let us know</h3>

        <ul className="list-unstyled">
          <li>
            <a
              className="typeform-share button"
              href="https://sung8.typeform.com/to/CTeDKi"
              data-mode="popup"
              target="_blank"
            >
              <i className="fa fa-external-link link-icon" />
              Write a short feedback
            </a>
          </li>
          <li>
            <a
              href="https://github.com/dgraph-io/dgraph/issues/new"
              target="_blank"
            >
              <i className="fa fa-external-link link-icon" />
              File a GitHub issue
            </a>
          </li>
        </ul>
      </section>
    </div>
  );
};
export default SidebarFeedback;

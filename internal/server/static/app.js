(function () {
  // Theme management
  var THEME_KEY = "bm-theme";

  function applyTheme(theme) {
    document.documentElement.setAttribute("data-theme", theme);
  }

  function savedTheme() {
    return localStorage.getItem(THEME_KEY) || "light";
  }

  function toggleTheme() {
    var current = document.documentElement.getAttribute("data-theme") || "light";
    var next = current === "dark" ? "light" : "dark";
    localStorage.setItem(THEME_KEY, next);
    applyTheme(next);
    updateThemeButton();
  }

  function updateThemeButton() {
    var btn = document.getElementById("theme-toggle");
    if (!btn) return;
    var theme = document.documentElement.getAttribute("data-theme") || "light";
    btn.textContent = theme === "dark" ? "☀️" : "🌙";
    btn.title = theme === "dark" ? "Switch to light mode" : "Switch to dark mode";
  }

  // Apply saved theme immediately
  applyTheme(savedTheme());

  document.addEventListener("DOMContentLoaded", function () {
    updateThemeButton();

    var themeBtn = document.getElementById("theme-toggle");
    if (themeBtn) {
      themeBtn.addEventListener("click", toggleTheme);
    }

    // Add project form toggle
    var addBtn = document.getElementById("add-project-btn");
    var addForm = document.getElementById("add-project-form");
    if (addBtn && addForm) {
      addBtn.addEventListener("click", function () {
        addForm.classList.toggle("visible");
        if (addForm.classList.contains("visible")) {
          var inp = addForm.querySelector("input[name='name']");
          if (inp) inp.focus();
        }
      });
    }

    // Token copy button
    var copyBtn = document.getElementById("copy-token-btn");
    if (copyBtn) {
      copyBtn.addEventListener("click", function () {
        var tokenEl = document.getElementById("new-token-value");
        if (!tokenEl) return;
        var text = tokenEl.textContent;
        navigator.clipboard.writeText(text).then(function () {
          copyBtn.textContent = "Copied!";
          setTimeout(function () { copyBtn.textContent = "Copy"; }, 2000);
        }).catch(function () {
          copyBtn.textContent = "Copy failed";
          setTimeout(function () { copyBtn.textContent = "Copy"; }, 2000);
        });
      });
    }

    // Shared project state helpers
    var PROJECT_STATE_KEY = "bm-project-state";

    function loadProjectStates() {
      try { return JSON.parse(localStorage.getItem(PROJECT_STATE_KEY)) || {}; } catch (e) { return {}; }
    }

    function saveProjectStates(states) {
      localStorage.setItem(PROJECT_STATE_KEY, JSON.stringify(states));
    }

    // Shared status/HTML helpers (used by both feature and dashboard live update)
    function statusLabel(s) {
      var m = {
        awaiting_human: 'Awaiting Human',
        awaiting_client: 'Awaiting Client',
        draft: 'Draft',
        fully_specified: 'Fully Specified',
        waiting: 'Waiting',
        ready_to_generate: 'Ready to Generate',
        generating: 'Generating',
        beads_created: 'Beads Created',
        done: 'Done',
        halted: 'Halted',
        abandoned: 'Abandoned',
        archived: 'Archived'
      };
      return m[s] || s;
    }

    function statusBadgeClass(s) {
      var c = {
        awaiting_human: 'badge-awaiting-human',
        awaiting_client: 'badge-awaiting-client',
        draft: 'badge-draft',
        fully_specified: 'badge-fully-specified',
        done: 'badge-done'
      };
      return c[s] || 'badge-default';
    }

    function escHTML(s) {
      return String(s)
        .replace(/&/g, '&amp;')
        .replace(/</g, '&lt;')
        .replace(/>/g, '&gt;')
        .replace(/"/g, '&quot;');
    }

    function formatLocalTime(isoString) {
      try {
        var d = new Date(isoString);
        var fmt = new Intl.DateTimeFormat(undefined, {
          year: 'numeric',
          month: '2-digit',
          day: '2-digit',
          hour: '2-digit',
          minute: '2-digit',
          hour12: false,
          timeZoneName: 'short'
        });
        var parts = fmt.formatToParts(d);
        var p = {};
        for (var i = 0; i < parts.length; i++) {
          p[parts[i].type] = parts[i].value;
        }
        return p.year + '-' + p.month + '-' + p.day + ' ' + p.hour + ':' + p.minute + ' ' + p.timeZoneName;
      } catch (e) {
        return null;
      }
    }

    function applyLocalTimes(container) {
      var els = container.querySelectorAll('time.local-time');
      for (var i = 0; i < els.length; i++) {
        var formatted = formatLocalTime(els[i].getAttribute('datetime'));
        if (formatted !== null) els[i].textContent = formatted;
      }
    }

    // Live page detection
    var livePage = document.querySelector('[data-live-page]');

    // Feature live update
    if (livePage && livePage.dataset.livePage === 'feature') {
      var projectName = livePage.dataset.project;
      var featureID = livePage.dataset.featureId;
      var isViewer = livePage.dataset.isViewer === 'true';

      var liveStatus = document.getElementById('live-status');
      var secondsAgo = 0;
      if (liveStatus) {
        liveStatus.style.display = '';
        liveStatus.textContent = 'Updated just now';
        setInterval(function () {
          secondsAgo++;
          liveStatus.textContent = 'Updated ' + secondsAgo + ' second' + (secondsAgo === 1 ? '' : 's') + ' ago';
        }, 1000);
      }

      var hasOpened = false;
      var es = new EventSource('/project/' + projectName + '/feature/' + featureID + '/events');
      es.addEventListener('update', function () { fetchFeature(); });
      es.onopen = function () {
        if (hasOpened) {
          fetchFeature();
        }
        hasOpened = true;
      };

      function fetchFeature() {
        fetch('/project/' + projectName + '/feature/' + featureID + '/data', { cache: 'no-store' })
          .then(function (r) { return r.json(); })
          .then(function (data) {
            secondsAgo = 0;
            if (liveStatus) liveStatus.textContent = 'Updated just now';
            patchFeature(data);
          })
          .catch(function () {
            if (liveStatus) liveStatus.textContent = 'Live update failed';
          });
      }

      function buildActionHTML(data) {
        var proj = escHTML(projectName);
        var fid = escHTML(featureID);
        var base = '/project/' + proj + '/feature/' + fid;
        var s = data.status;

        if (s === 'draft') {
          if (isViewer) return '';
          return '<div class="card"><h2>Actions</h2>' +
            '<details style="margin-bottom:1rem"><summary style="cursor:pointer;font-weight:600">Edit Description</summary>' +
            '<form method="post" action="' + base + '/description" style="margin-top:0.75rem">' +
            '<textarea name="description" rows="10" style="width:100%;box-sizing:border-box"></textarea>' +
            '<div style="margin-top:0.5rem"><button type="submit" class="btn">Save Description</button></div>' +
            '</form></details>' +
            '<form method="post" action="' + base + '/start-dialog">' +
            '<button type="submit" class="btn btn-primary">Start Dialog</button></form>' +
            '<form method="post" action="' + base + '/delete" style="margin-top:1rem">' +
            '<button type="submit" class="btn btn-danger" onclick="return confirm(\'Permanently delete this draft feature? This cannot be undone.\')">Delete Draft</button></form>' +
            '</div>';
        }

        if (s === 'awaiting_human') {
          var iters = data.iterations || [];
          var last = iters.length > 0 ? iters[iters.length - 1] : null;
          var questionsHTML = '';
          if (last && last.questions) {
            questionsHTML = '<div style="background:var(--card-bg);border:1px solid var(--border);border-radius:6px;padding:1rem;margin-bottom:1rem">' +
              '<div style="font-weight:600;margin-bottom:0.5rem">Client\'s Questions</div>' +
              '<div class="md-content"><pre style="white-space:pre-wrap;margin:0">' + escHTML(last.questions) + '</pre></div>' +
              '</div>';
          }
          if (isViewer) {
            return '<div class="card"><h2>Respond to Client</h2>' + questionsHTML + '</div>';
          }
          return '<div class="card"><h2>Respond to Client</h2>' + questionsHTML +
            '<form method="post" action="' + base + '/respond">' +
            '<div style="margin-bottom:0.75rem">' +
            '<label for="response" style="font-weight:600;display:block;margin-bottom:0.25rem">Your Response</label>' +
            '<textarea name="response" id="response" rows="8" style="width:100%;box-sizing:border-box"></textarea>' +
            '</div>' +
            '<div style="margin-bottom:0.75rem">' +
            '<label style="display:flex;align-items:center;gap:0.5rem;cursor:pointer">' +
            '<input type="checkbox" name="final" value="on"> Final answer &mdash; no further questions needed' +
            '</label></div>' +
            '<div class="flex-row"><button type="submit" class="btn btn-primary">Send Response</button></div>' +
            '</form></div>';
        }

        if (s === 'awaiting_client') {
          return '<div class="card"><div class="muted" style="text-align:center;padding:0.5rem 0">&#8987; Client is processing&hellip;</div></div>';
        }

        if (s === 'fully_specified') {
          if (isViewer) return '';
          var otherFeatures = data.other_features || [];
          var genAfterHTML = '';
          if (otherFeatures.length > 0) {
            var opts = '<option value="">Generate After&hellip;</option>';
            for (var i = 0; i < otherFeatures.length; i++) {
              var f = otherFeatures[i];
              opts += '<option value="' + escHTML(f.id) + '">' + escHTML(f.name) + ' (' + escHTML(f.id) + ')</option>';
            }
            genAfterHTML = '<form method="post" action="' + base + '/generate-after" style="display:inline;margin-left:0.5rem">' +
              '<select name="after_feature_id" style="margin-right:0.25rem">' + opts + '</select>' +
              '<button type="submit" class="btn">Set Dependency</button></form>';
          }
          return '<div class="card"><h2>Actions</h2>' +
            '<details style="margin-bottom:1rem"><summary style="cursor:pointer;font-weight:600">Reopen Dialog</summary>' +
            '<form method="post" action="' + base + '/reopen" style="margin-top:0.75rem">' +
            '<div style="margin-bottom:0.5rem">' +
            '<label for="reopen-message" style="font-weight:600;display:block;margin-bottom:0.25rem">Message (optional)</label>' +
            '<textarea name="message" id="reopen-message" rows="4" style="width:100%;box-sizing:border-box"></textarea>' +
            '</div>' +
            '<button type="submit" class="btn">Reopen</button></form></details>' +
            '<div class="flex-row" style="margin-top:0.25rem;align-items:center;gap:0.5rem">' +
            '<form method="post" action="' + base + '/generate-now" style="display:inline">' +
            '<button type="submit" class="btn btn-primary">Generate Now</button>' +
            '</form>' + genAfterHTML + '</div></div>';
        }

        if (s === 'waiting' || s === 'ready_to_generate' || s === 'generating') {
          return '<div class="card"><div class="muted" style="margin-bottom:0.5rem">Status: <strong>' +
            statusLabel(s) + '</strong></div><button type="button" class="btn" disabled>Halt</button></div>';
        }

        if (s === 'done') {
          if (isViewer) return '';
          return '<div class="card"><div class="muted" style="margin-bottom:0.5rem">Status: <strong>' +
            statusLabel(s) + '</strong></div>' +
            '<form method="post" action="' + base + '/archive">' +
            '<button type="submit" class="btn">Archive</button></form></div>';
        }

        if (s === 'beads_created') {
          var bp = data.bead_progress;
          var progressHTML = '';
          if (bp) {
            if (bp.unavailable) {
              progressHTML = '<div class="muted" style="margin-bottom:0.75rem">Bead status unavailable</div>';
            } else {
              var pct = bp.total > 0 ? Math.floor(bp.closed * 100 / bp.total) : 0;
              progressHTML = '<div style="margin-bottom:0.75rem">' +
                '<strong id="bead-progress-label">' + escHTML(String(bp.closed)) + '/' + escHTML(String(bp.total)) + ' beads closed</strong>' +
                '<div style="background:var(--border);border-radius:4px;height:8px;margin-top:0.4rem;overflow:hidden">' +
                (bp.total > 0 ? '<div id="bead-progress-fill" style="background:var(--accent,#4caf50);height:100%;width:' + pct + '%"></div>' : '') +
                '</div></div>';
            }
          }
          return '<div class="card"><div class="muted" style="margin-bottom:0.5rem">Status: <strong>' +
            statusLabel(s) + '</strong></div>' + progressHTML +
            '<button type="button" class="btn" disabled>Halt</button></div>';
        }

        return '';
      }

      function buildRoundHTML(iter) {
        var html = '<details' + (iter.is_last ? ' open' : '') + ' style="margin-bottom:0.75rem">' +
          '<summary style="cursor:pointer;font-weight:600;padding:0.5rem 0">Round ' + escHTML(String(iter.round)) +
          (iter.is_final ? ' &mdash; <span class="badge badge-fully-specified" style="font-size:0.75rem">Final</span>' : '') +
          '</summary><div class="card" style="margin-top:0.5rem">';
        if (iter.description) {
          html += '<div class="iteration-block"><div class="iteration-label">Revised Description</div>' +
            '<div class="md-content"><pre style="white-space:pre-wrap;margin:0">' + escHTML(iter.description) + '</pre></div></div>';
        }
        if (iter.questions) {
          html += '<div class="iteration-block"><div class="iteration-label">Client Questions</div>' +
            '<div class="md-content"><pre style="white-space:pre-wrap;margin:0">' + escHTML(iter.questions) + '</pre></div></div>';
        }
        if (iter.response) {
          html += '<div class="iteration-block"><div class="iteration-label">Human Response</div>' +
            '<div class="md-content"><pre style="white-space:pre-wrap;margin:0">' + escHTML(iter.response) + '</pre></div></div>';
        }
        html += '</div></details>';
        return html;
      }

      function patchFeature(data) {
        // Update feature title and rename form input
        var titleEl = livePage.querySelector('h1');
        if (titleEl && data.name) {
          titleEl.textContent = data.name;
        }
        var renameInput = livePage.querySelector('input[name="name"]');
        if (renameInput && data.name && renameInput !== document.activeElement) {
          renameInput.value = data.name;
        }

        // Update status badge
        var badge = livePage.querySelector('.badge');
        if (badge) {
          badge.textContent = statusLabel(data.status);
          badge.className = 'badge ' + statusBadgeClass(data.status);
        }

        // Update action section (skip if user is interacting with a form field)
        var actionSection = document.getElementById('action-section');
        if (actionSection) {
          var inputs = actionSection.querySelectorAll('textarea, input');
          var skip = false;
          for (var i = 0; i < inputs.length; i++) {
            if (inputs[i] === document.activeElement || inputs[i].value !== '') {
              skip = true;
              break;
            }
          }
          if (!skip) {
            actionSection.innerHTML = buildActionHTML(data);
          }
        }

        // Update bead progress (if element exists independently of action section replacement)
        if (data.bead_progress && !data.bead_progress.unavailable && data.status === 'beads_created') {
          var label = document.getElementById('bead-progress-label');
          var fill = document.getElementById('bead-progress-fill');
          if (label) {
            label.textContent = data.bead_progress.closed + '/' + data.bead_progress.total + ' beads closed';
          }
          if (fill && data.bead_progress.total > 0) {
            fill.style.width = Math.floor(data.bead_progress.closed * 100 / data.bead_progress.total) + '%';
          }
        }

        // Append new dialog rounds
        var roundsContainer = document.getElementById('dialog-rounds');
        if (roundsContainer && data.iterations) {
          var existing = roundsContainer.querySelectorAll('details').length;
          if (data.iterations.length > existing) {
            if (existing === 0) {
              var h2 = document.createElement('h2');
              h2.style.marginTop = '1.5rem';
              h2.textContent = 'Dialog History';
              roundsContainer.appendChild(h2);
            }
            for (var r = existing; r < data.iterations.length; r++) {
              var tmp = document.createElement('div');
              tmp.innerHTML = buildRoundHTML(data.iterations[r]);
              roundsContainer.appendChild(tmp.firstChild);
            }
          }
        }
      }

      fetchFeature();
    }

    // Dashboard live update
    if (livePage && livePage.dataset.livePage === 'dashboard') {
      var dashLiveStatus = document.getElementById('live-status');
      var dashSecondsAgo = 0;
      if (dashLiveStatus) {
        dashLiveStatus.style.display = '';
        dashLiveStatus.textContent = 'Updated 0s ago';
        setInterval(function () {
          dashSecondsAgo++;
          dashLiveStatus.textContent = 'Updated ' + dashSecondsAgo + 's ago';
        }, 1000);
      }

      var dashEsOpened = false;
      var dashEs = new EventSource('/events');
      dashEs.addEventListener('update', function () { fetchDashboard(); });
      dashEs.onopen = function () {
        if (dashEsOpened) {
          fetchDashboard();
        }
        dashEsOpened = true;
      };

      function fetchDashboard() {
        fetch('/data', { cache: 'no-store' })
          .then(function (r) {
            if (!r.ok) throw new Error('not ok');
            return r.json();
          })
          .then(function (data) {
            dashSecondsAgo = 0;
            if (dashLiveStatus) dashLiveStatus.textContent = 'Updated 0s ago';
            patchDashboard(data);
          })
          .catch(function () {
            if (dashLiveStatus) dashLiveStatus.textContent = 'Update failed \u2014 retrying';
          });
      }

      var isViewer = !!document.querySelector('.viewer-badge');

      function buildProjectBodyHTML(p) {
        var proj = escHTML(p.name);
        if (!p.features || p.features.length === 0) {
          return '<div class="project-body"><p class="muted">No features yet. <a href="/project/' + proj + '/new">Create one</a>.</p></div>';
        }
        var rows = '';
        for (var i = 0; i < p.features.length; i++) {
          var f = p.features[i];
          var beadHTML = f.bead_info
            ? '<span class="muted" style="font-size:0.8rem;margin-left:0.4rem">' + escHTML(f.bead_info) + '</span>'
            : '';
          var archiveHTML = (f.status === 'done' && !isViewer)
            ? '<form method="post" action="/project/' + proj + '/feature/' + escHTML(f.id) + '/archive" style="display:inline;margin-left:0.4rem"><button type="submit" class="btn" style="padding:0.1rem 0.4rem;font-size:0.75rem">Archive</button></form>'
            : '';
          rows += '<tr>' +
            '<td><a href="/project/' + proj + '/feature/' + escHTML(f.id) + '">' + escHTML(f.name) + '</a></td>' +
            '<td><span class="badge ' + escHTML(statusBadgeClass(f.status)) + '">' + escHTML(statusLabel(f.status)) + '</span>' + beadHTML + archiveHTML + '</td>' +
            '<td class="muted" style="font-size:0.85rem;white-space:nowrap"><time class="local-time" datetime="' + escHTML(f.updated_at_iso) + '">' + escHTML(f.updated_at) + '</time></td>' +
            '</tr>';
        }
        return '<div class="project-body"><table class="feature-table"><thead><tr><th>Name</th><th>Status</th><th>Updated</th></tr></thead><tbody>' + rows + '</tbody></table></div>';
      }

      function attachToggleListener(el, name) {
        if (el._toggleHandler) {
          el.removeEventListener('toggle', el._toggleHandler);
        }
        el._toggleHandler = function () {
          var s = loadProjectStates();
          s[name] = el.open;
          saveProjectStates(s);
        };
        el.addEventListener('toggle', el._toggleHandler);
      }

      // Restores el.open from states[name], defaulting to open on first visit.
      // Returns true if states was mutated (new entry added).
      function applyOpenState(el, name, states) {
        if (Object.prototype.hasOwnProperty.call(states, name)) {
          el.open = states[name];
          return false;
        }
        el.open = true;
        states[name] = true;
        return true;
      }

      function patchDashboard(data) {
        var states = loadProjectStates();
        var statesChanged = false;
        var projectNames = Object.create(null);

        for (var i = 0; i < data.projects.length; i++) {
          projectNames[data.projects[i].name] = true;
        }

        // Snapshot existing cards once; track insertion point for newly appended cards.
        var existingCards = document.querySelectorAll('.project-card');
        var insertAfter = existingCards.length > 0 ? existingCards[existingCards.length - 1] : null;

        for (var j = 0; j < data.projects.length; j++) {
          var p = data.projects[j];
          var el = document.querySelector('details[data-project="' + p.name + '"]');

          if (el) {
            // Replace project body content
            var bodyDiv = el.querySelector('.project-body');
            var tmp = document.createElement('div');
            tmp.innerHTML = buildProjectBodyHTML(p);
            var newBodyEl = tmp.firstChild;
            if (bodyDiv) {
              bodyDiv.parentNode.replaceChild(newBodyEl, bodyDiv);
            } else {
              el.appendChild(newBodyEl);
            }
            applyLocalTimes(newBodyEl);

            // Update connectivity badge in summary
            var connSpan = el.querySelector('summary .connectivity');
            if (connSpan) {
              if (p.connectivity) {
                var isConnected = p.connectivity.indexOf('Connected') === 0;
                connSpan.className = 'connectivity' + (isConnected ? ' connected' : '');
                connSpan.textContent = '\u25cf ' + p.connectivity;
              } else {
                connSpan.className = 'connectivity muted';
                connSpan.textContent = '\u25cb Never connected';
              }
            }

            // Restore open state from localStorage (default open on first visit)
            if (applyOpenState(el, p.name, states)) statesChanged = true;

            // Re-attach toggle listener
            attachToggleListener(el, p.name);
          } else {
            // Append new project card
            var isConn = p.connectivity && p.connectivity.indexOf('Connected') === 0;
            var connHTML = p.connectivity
              ? '<span class="connectivity' + (isConn ? ' connected' : '') + '" style="margin-left:0.5rem">\u25cf ' + escHTML(p.connectivity) + '</span>'
              : '<span class="connectivity muted" style="margin-left:0.5rem">\u25cb Never connected</span>';
            var featureCountText = p.feature_count === 1 ? '1 feature' : (p.feature_count + ' features');
            var cardHTML = '<div class="card project-card">' +
              '<details data-project="' + escHTML(p.name) + '">' +
              '<summary>' +
              '<span style="font-weight:600;font-size:1.1rem">' + escHTML(p.name) + '</span>' +
              '<span class="muted" style="font-size:0.85rem;margin-left:0.5rem">' + escHTML(featureCountText) + '</span>' +
              connHTML +
              '<span class="spacer"></span>' +
              '<a class="btn btn-secondary btn-sm" href="/project/' + escHTML(p.name) + '/new" onclick="event.stopPropagation()">+ New Feature</a>' +
              '</summary>' +
              buildProjectBodyHTML(p) +
              '</details></div>';

            var cardTmp = document.createElement('div');
            cardTmp.innerHTML = cardHTML;
            var newCard = cardTmp.firstChild;

            if (insertAfter) {
              insertAfter.parentNode.insertBefore(newCard, insertAfter.nextSibling);
            } else {
              document.querySelector('main').appendChild(newCard);
            }
            insertAfter = newCard;
            applyLocalTimes(newCard);

            var newDetails = newCard.querySelector('details[data-project]');
            if (newDetails) {
              if (applyOpenState(newDetails, p.name, states)) statesChanged = true;
              attachToggleListener(newDetails, p.name);
            }
          }
        }

        if (statesChanged) saveProjectStates(states);

        // Remove stale project cards
        var existingDetails = document.querySelectorAll('details[data-project]');
        for (var k = 0; k < existingDetails.length; k++) {
          var detailName = existingDetails[k].getAttribute('data-project');
          if (!projectNames[detailName]) {
            var card = existingDetails[k].closest('.project-card');
            if (card) {
              card.parentNode.removeChild(card);
            } else {
              existingDetails[k].parentNode.removeChild(existingDetails[k]);
            }
          }
        }
      }

      fetchDashboard();
    }

    // Format local-time elements on initial page load
    document.querySelectorAll('time.local-time').forEach(function (el) {
      var iso = el.getAttribute('datetime');
      var formatted = formatLocalTime(iso);
      if (formatted !== null) el.textContent = formatted;
    });

    // Project expand/collapse persistence (non-dashboard pages; dashboard is handled by patchDashboard)
    if (!livePage || livePage.dataset.livePage !== 'dashboard') {
      var details = document.querySelectorAll("details[data-project]");
      var states = loadProjectStates();
      var changed = false;

      details.forEach(function (el) {
        var name = el.getAttribute("data-project");
        if (Object.prototype.hasOwnProperty.call(states, name)) {
          el.open = states[name];
        } else {
          el.open = true;
          states[name] = true;
          changed = true;
        }
        el.addEventListener("toggle", function () {
          var s = loadProjectStates();
          s[name] = el.open;
          saveProjectStates(s);
        });
      });
      if (changed) saveProjectStates(states);
    }
  });
})();

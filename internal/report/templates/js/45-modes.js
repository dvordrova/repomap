// Two entrances, one report. Maps stay mounted: changing reading mode must
// never clear a selected operation, the scope, search, zoom or reading path.
(function(){
  var body=document.body, bar=document.querySelector('.reading-modes');
  var pages=Array.from(document.querySelectorAll('main > [data-report-page]'));
  var home=document.getElementById('overview'), repoMap=document.getElementById('repository-map');
  if(!bar||!home)return;
  var toolbar=bar.closest('.report-toolbar');
  var returnLinks=document.createElement('div');returnLinks.className='reading-return';returnLinks.hidden=true;toolbar.appendChild(returnLinks);
  var returnLink=document.createElement('a'),termLink=document.createElement('a'),mapLink=document.createElement('a');returnLinks.append(returnLink,termLink,mapLink);
  var detailGroup=null;
  function measureToolbar(){document.documentElement.style.setProperty('--toolbar-height',getComputedStyle(toolbar).position==='sticky'?toolbar.getBoundingClientRect().height+'px':'0px');}
  new ResizeObserver(measureToolbar).observe(toolbar);measureToolbar();
  var current=home, mode='learn', question=null, term=null;
  function enclosing(node){return node&&node.closest('[data-report-page]');}
  function locate(){try{return document.getElementById(decodeURIComponent(location.hash.slice(1)));}catch(e){return null;}}
  function showReturn(){
    returnLink.hidden=!question||current===enclosing(question);
    if(question){returnLink.href='#'+question.id;returnLink.textContent='← Back to question: '+question.querySelector('.reading-question').textContent;}
    termLink.hidden=!term||current===enclosing(term);
    if(term){termLink.href='#'+term.id;termLink.textContent='← Back to term: '+term.querySelector('summary').textContent;}
    var map=detailGroup&&current.querySelector('[data-map-explorer]');
    mapLink.hidden=!map;
    if(map){mapLink.href='#'+current.id;mapLink.textContent='← Back to map: '+map.explorationLabel();}
    returnLinks.hidden=returnLink.hidden&&termLink.hidden&&mapLink.hidden;
  }
  function setPage(node){
    var page=enclosing(node);if(!page)return;
    var nextQuestion=node.closest('.reading-guide');
    if(nextQuestion){question=nextQuestion;term=null;}
    else if(page===home||node.id==='questions'){question=null;term=null;}
    var nextTerm=node.closest('.learn-concept');
    if(nextTerm)term=nextTerm;
    else if(node.id==='concepts')term=null;
    revealConcept(node);
    current=page;detailGroup=node.closest('.group');pages.forEach(function(p){p.hidden=p!==page;});
    showReturn();
    var place=page===home?'':(page.dataset.componentName||page.querySelector('h2')?.textContent||'');
    var detailTitle=detailGroup&&detailGroup.querySelector('.group-head h4').cloneNode(true);
    if(detailTitle)detailTitle.querySelectorAll('button,.model-sources').forEach(function(n){n.remove();});
    document.querySelector('.reading-location').textContent=document.querySelector('.brand .repo').textContent+(place?' · '+place:'')+(detailTitle?' · Full details: '+detailTitle.textContent:'');
    for(var p=node;p&&p!==page;p=p.parentElement)if(p.tagName==='DETAILS')p.open=true;
    document.querySelectorAll('.target-picker').forEach(function(p){p.open=false;});
    // Navigation scrolls immediately; the return links and search panel may
    // have changed this height before ResizeObserver gets its next turn.
    measureToolbar();
  }
  function address(node,replace){
    if(!node.id)node=enclosing(node);
    var url=new URL(location.href);url.searchParams.set('mode',mode);url.hash=node.id;
    if(url.href!==location.href)history[replace?'replaceState':'pushState'](history.state,'',url);
  }
  function setMode(next,record){
    mode=next==='work'?'work':'learn';body.dataset.readingMode=mode;
    document.querySelectorAll('[data-reading-mode]').forEach(function(el){if(el!==body)el.hidden=el.dataset.readingMode!==mode;});
    bar.querySelectorAll('[data-mode]').forEach(function(b){b.setAttribute('aria-pressed',b.dataset.mode===mode);});
    // A single-component report has no repository SVG; its existing card
    // remains a direct entrance instead of an empty Work home.
    if(!repoMap.querySelector('.repo-map'))repoMap.querySelector('.evidence-list').open=true;
    if(record){var url=new URL(location.href);url.searchParams.set('mode',mode);history.pushState(history.state,'',url);}
  }
  // All navigators (search, map deep links and ordinary anchors) reveal the
  // existing section before any map measures its available space.
  document.addEventListener('repomap:navigate',function(e){var node=e.detail.destination;if(!enclosing(node))return;setPage(node);address(node);});
  document.addEventListener('click',function(e){
    if(e.target.closest('a')===mapLink&&!e.ctrlKey&&!e.metaKey&&!e.shiftKey&&!e.altKey){
      e.preventDefault();var map=current.querySelector('[data-map-explorer]');
      setPage(map);address(current);map.resumeExploration();map.scrollIntoView({block:'start'});return;
    }
    var readingQuestion=e.target.closest('.reading-guide');
    if(readingQuestion){question=readingQuestion;term=null;showReturn();}
    var a=e.target.closest('a[href^="#"]');if(!a||a.closest('[data-map-explorer]')||a.hasAttribute('data-open')||e.ctrlKey||e.metaKey||e.shiftKey||e.altKey)return;
    var node=document.getElementById(a.getAttribute('href').slice(1));if(!enclosing(node))return;
    e.preventDefault();setPage(node);address(node);
    // Map-node links are handled by their existing explorer. Scrolling an
    // SVG node directly would pan the page to an offscreen graph coordinate.
    if(!node.matches('[data-node]'))node.scrollIntoView({block:'start'});
  },true);
  bar.querySelectorAll('[data-mode]').forEach(function(b){b.addEventListener('click',function(){setMode(b.dataset.mode,true);});});
  function restore(){setMode(new URL(location.href).searchParams.get('mode'),false);setPage(locate()||home);}
  window.addEventListener('popstate',restore);window.addEventListener('hashchange',restore);
  bar.hidden=false;body.classList.add('reading-ready');restore();
  document.querySelectorAll('.learn-band').forEach(function(band){band.addEventListener('toggle',function(){if(!band.open)return;document.querySelectorAll('.learn-band').forEach(function(other){if(other!==band)other.open=false;});});});

  var library=document.getElementById('concepts');if(!library)return;
  var search=library.querySelector('input'), entries=Array.from(library.querySelectorAll('.learn-concept'));
  var pager=library.querySelector('.concept-pages'), count=library.querySelector('.concept-count'), page=0, size=8;
  var text=entries.map(function(entry){return entry.textContent.toLowerCase();});
  function revealConcept(node){
    if(!library||!entries||!node)return;
    var entry=node.closest('.learn-concept'),index=entries.indexOf(entry);
    if(index<0)return;
    // A source-linked term may be outside the current search/page. Reveal
    // that exact entry before the common navigator scrolls to its anchor.
    search.value='';page=Math.floor(index/size);filter();entry.open=true;
  }
  function filter(){
    var terms=search.value.toLowerCase().trim().split(/\s+/), found=entries.filter(function(e,i){return terms.every(function(t){return text[i].includes(t);});});
    page=Math.min(page,Math.max(0,Math.ceil(found.length/size)-1));
    var visible=new Set(found.slice(page*size,(page+1)*size));entries.forEach(function(e){e.hidden=!visible.has(e);});
    count.textContent=found.length?found.length+' terms':'No terms match. Try another word from its name or description.';
    pager.hidden=found.length<=size;pager.querySelector('span').textContent=(found.length?page*size+1:0)+'–'+Math.min((page+1)*size,found.length)+' of '+found.length;
    pager.querySelector('[data-prev]').disabled=page===0;pager.querySelector('[data-next]').disabled=(page+1)*size>=found.length;
  }
  library.querySelector('.concept-search').hidden=false;
  search.addEventListener('input',function(){page=0;filter();});
  pager.querySelector('[data-prev]').addEventListener('click',function(){page--;filter();library.scrollIntoView({block:'start'});});
  pager.querySelector('[data-next]').addEventListener('click',function(){page++;filter();library.scrollIntoView({block:'start'});});filter();revealConcept(locate());
})();

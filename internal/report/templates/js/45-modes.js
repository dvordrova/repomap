// Two entrances, one report. Maps stay mounted: changing reading mode must
// never clear a selected operation, the scope, search, zoom or reading path.
(function(){
  var body=document.body, bar=document.querySelector('.reading-modes');
  var pages=Array.from(document.querySelectorAll('main > [data-report-page]'));
  var home=document.getElementById('overview'), repoMap=document.getElementById('repository-map');
  if(!bar||!home)return;
  var toolbar=bar.closest('.report-toolbar');
  var locationLine=toolbar.querySelector('.reading-location'),locationName=document.createElement('span'),mapContext=document.createElement('a');
  mapContext.className='reading-map-context';mapContext.dataset.readingMapReturn='';mapContext.hidden=true;locationLine.append(locationName,' ',mapContext);
  var returnLinks=document.createElement('div');returnLinks.className='reading-return';returnLinks.hidden=true;toolbar.appendChild(returnLinks);
  var returnLink=document.createElement('a'),termLink=document.createElement('a'),mapLink=document.createElement('a');mapLink.dataset.readingMapReturn='';returnLinks.append(returnLink,termLink,mapLink);
  var detailGroup=null;
  function measureToolbar(){document.documentElement.style.setProperty('--toolbar-height',getComputedStyle(toolbar).position==='sticky'?toolbar.getBoundingClientRect().height+'px':'0px');}
  new ResizeObserver(measureToolbar).observe(toolbar);measureToolbar();
  var current=home, mode='learn', question=null, term=null;
  var questionPage=document.getElementById('questions'),guides=questionPage?Array.from(questionPage.querySelectorAll('.reading-guide')):[];
  var questionIndex=questionPage&&questionPage.querySelector('.question-index');
  function enclosing(node){return node&&node.closest('[data-report-page]');}
  function locate(){try{return document.getElementById(decodeURIComponent(location.hash.slice(1)));}catch(e){return null;}}
  function selectQuestion(selected){
    guides.forEach(function(guide){guide.hidden=guide!==selected;guide.open=guide===selected;});
    if(questionIndex)questionIndex.open=!selected;
    document.querySelectorAll('.question-menu a').forEach(function(a){
      if(selected&&a.getAttribute('href')==='#'+selected.id)a.setAttribute('aria-current','true');else a.removeAttribute('aria-current');
    });
  }
  function textElement(tag,text,className){var el=document.createElement(tag);el.textContent=text;if(className)el.className=className;return el;}
  function questionLink(guide){var a=textElement('a',guide.querySelector('.reading-question').textContent);a.href='#'+guide.id;return a;}
  function prepareReadingEntrances(){
    guides.forEach(function(guide,index){
      var position=document.createElement('nav');position.className='question-position';position.setAttribute('aria-label',rmT('Question navigation'));
      position.appendChild(textElement('span',rmT('Question {0} of {1}',index+1,guides.length)));
      var all=textElement('a',rmT('← All questions'));all.href='#questions';position.appendChild(all);
      if(index+1<guides.length){var next=textElement('a',rmT('Next question →'));next.href='#'+guides[index+1].id;position.appendChild(next);}
      guide.querySelector('.reading-question').after(position);
    });
    pages.filter(function(page){return page.hasAttribute('data-component-name');}).forEach(function(page){
      var map=page.querySelector('[data-map-explorer]');if(!map)return;
      // Only the answer's existing exact map links establish this menu. It
      // does not guess relevance from names, proximity or the selected node.
      var linked=guides.filter(function(guide){return Array.from(guide.querySelectorAll('[data-question-map]')).some(function(a){
        var node=document.getElementById(a.dataset.questionMap||(a.getAttribute('href')||'').slice(1));return enclosing(node)===page;
      });});
      var learn=document.createElement('aside');learn.className='component-reading-entry';learn.dataset.readingMode='learn';
      learn.appendChild(textElement('strong',rmT('Learn: questions linked to this component')));
      if(linked.length){
        var menu=document.createElement('details');menu.className='component-question-menu';
        menu.appendChild(textElement('summary',rmT('{0} questions link here',linked.length)));
        var list=document.createElement('ol');list.className='question-menu';linked.forEach(function(guide){var li=document.createElement('li');li.appendChild(questionLink(guide));list.appendChild(li);});menu.appendChild(list);learn.appendChild(menu);
      }else learn.appendChild(textElement('span',rmT('No saved question links to this component.')));
      var all=textElement('a',rmT('All questions'));all.href='#questions';if(guides.length)learn.appendChild(all);
      var work=document.createElement('aside');work.className='component-reading-entry';work.dataset.readingMode='work';
      work.appendChild(textElement('strong',rmT('Work: investigate a place in the code')));
      var button=textElement('button',rmT('Find in repository'),'reading-search-entry');button.type='button';button.dataset.readingFind='';
      var component=document.querySelector('[data-find-component]');
      if(component&&Array.from(component.options).some(function(option){return option.value===page.id;})){button.dataset.readingFind=page.id;button.textContent=rmT('Find in this component');}
      // Map navigation scrolls to the figure itself. Keep the reading
      // entrance inside that figure so the sticky toolbar cannot cover it.
      work.appendChild(button);map.prepend(learn,work);
    });
    document.querySelectorAll('[data-reading-find]').forEach(function(button){button.hidden=false;});
  }
  function showReturn(){
    returnLink.hidden=!question||current===enclosing(question);
    if(question){returnLink.href='#'+question.id;returnLink.textContent=rmT('← Back to question: {0}',question.querySelector('.reading-question').textContent);}
    termLink.hidden=!term||current===enclosing(term);
    if(term){termLink.href='#'+term.id;termLink.textContent=rmT('← Back to term: {0}',term.querySelector('summary').textContent);}
    var map=detailGroup&&current.querySelector('[data-map-explorer]');
    mapLink.hidden=!map;
    if(map){mapLink.href='#'+mapDestination(map).id;mapLink.textContent=rmT('← Back to map: {0}',map.explorationLabel());}
    returnLinks.hidden=returnLink.hidden&&termLink.hidden&&mapLink.hidden;
  }
  function showLocation(){
    var place=current===home?'':(current.dataset.componentName||current.querySelector('h2')?.textContent||'');
    if(current===questionPage&&question)place=rmT('Question {0} of {1}',guides.indexOf(question)+1,guides.length)+' · '+question.querySelector('.reading-question').textContent;
    var detailTitle=detailGroup&&detailGroup.querySelector('.group-head h4').cloneNode(true);
    if(detailTitle)detailTitle.querySelectorAll('button,.model-sources').forEach(function(n){n.remove();});
    locationName.textContent=document.querySelector('.brand .repo').textContent+(place?' · '+place:'')+(detailTitle?' · '+rmT('Full details: {0}',detailTitle.textContent):'');
    var map=!detailGroup&&current.querySelector('[data-map-explorer]');
    mapContext.hidden=!map;
    if(map){mapContext.href='#'+mapDestination(map).id;mapContext.textContent=rmT('Map: {0}',map.explorationLabel());}
  }
  function mapDestination(map){return document.getElementById(map.explorerScope)||map.explorerOperation||current;}
  function setPage(node){
    var page=enclosing(node);if(!page)return;
    var nextQuestion=node.closest('.reading-guide');
    if(page===questionPage)selectQuestion(nextQuestion);
    if(nextQuestion){question=nextQuestion;term=null;}
    else if(page===home||node.id==='questions'){question=null;term=null;}
    var nextTerm=node.closest('.learn-concept');
    if(nextTerm)term=nextTerm;
    else if(node.id==='concepts')term=null;
    revealConcept(node);
    current=page;detailGroup=node.closest('.group');pages.forEach(function(p){p.hidden=p!==page;});
    showReturn();showLocation();
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
  // Layout commits the explorer's operation and scope. Hidden maps and the
  // inspector's temporary hover subject must not replace this reading context.
  document.addEventListener('repomap:layout',function(e){if(!detailGroup&&enclosing(e.target)===current)showLocation();},true);
  document.addEventListener('repomap:reading',function(e){if(!detailGroup&&enclosing(e.target)===current)showLocation();},true);
  document.addEventListener('click',function(e){
    var findAction=e.target.closest('[data-reading-find]');
    if(findAction){
      var search=document.querySelector('.nav .find'),component=document.querySelector('[data-find-component]');
      if(component){component.value=findAction.dataset.readingFind||'';component.dispatchEvent(new Event('change',{bubbles:true}));}
      if(search)search.focus({preventScroll:true});return;
    }
    if(e.target.closest('a')===mapContext&&!e.ctrlKey&&!e.metaKey&&!e.shiftKey&&!e.altKey){
      e.preventDefault();var contextMap=current.querySelector('[data-map-explorer]');contextMap.resumeExploration();contextMap.scrollIntoView({block:'start'});return;
    }
    if(e.target.closest('a')===mapLink&&!e.ctrlKey&&!e.metaKey&&!e.shiftKey&&!e.altKey){
      e.preventDefault();var map=current.querySelector('[data-map-explorer]');
      setPage(map);address(mapDestination(map));map.resumeExploration();map.scrollIntoView({block:'start'});return;
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
  prepareReadingEntrances();bar.hidden=false;body.classList.add('reading-ready');selectQuestion(null);restore();
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
    count.textContent=found.length?rmT('{0} terms',found.length):rmT('No terms match. Try another word from its name or description.');
    pager.hidden=found.length<=size;pager.querySelector('span').textContent=rmT('{0}–{1} of {2}',found.length?page*size+1:0,Math.min((page+1)*size,found.length),found.length);
    pager.querySelector('[data-prev]').disabled=page===0;pager.querySelector('[data-next]').disabled=(page+1)*size>=found.length;
  }
  library.querySelector('.concept-search').hidden=false;
  search.addEventListener('input',function(){page=0;filter();});
  pager.querySelector('[data-prev]').addEventListener('click',function(){page--;filter();library.scrollIntoView({block:'start'});});
  pager.querySelector('[data-next]').addEventListener('click',function(){page++;filter();library.scrollIntoView({block:'start'});});filter();revealConcept(locate());
})();
